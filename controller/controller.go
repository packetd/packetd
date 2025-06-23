// Copyright 2025 The packetd Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	"net/http"
	"strconv"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/connstream"
	"github.com/packetd/packetd/exporter"
	"github.com/packetd/packetd/internal/labels"
	"github.com/packetd/packetd/internal/metricstorage"
	"github.com/packetd/packetd/internal/wait"
	"github.com/packetd/packetd/logger"
	"github.com/packetd/packetd/pipeline"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/server"
	"github.com/packetd/packetd/sniffer"
)

type Config struct {
	Layer4Metrics struct {
		Enabled        bool     `config:"enabled"`
		RequiredLabels []string `config:"requiredLabels"`
	} `config:"layer4Metrics"`
}

type Controller struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	pl   *pipeline.Pipeline
	exp  *exporter.Exporter
	svr  *server.Server
	snif sniffer.Sniffer

	storage    *metricstorage.Storage
	roundtrips chan socket.RoundTrip

	ports map[socket.Port]socket.L7Proto
	pools map[socket.L7Proto]protocol.ConnPool
}

func setupLogger(conf *confengine.Config) error {
	var opts logger.Options
	if err := conf.UnpackChild("logger", &opts); err != nil {
		return err
	}

	if opts.Filename == "" {
		opts.Filename = "packetd.log"
	}
	if opts.MaxBackups <= 0 {
		opts.MaxBackups = 10
	}
	if opts.MaxAge <= 0 {
		opts.MaxAge = 7
	}
	if opts.MaxSize <= 0 {
		opts.MaxSize = 100
	}

	logger.SetOptions(opts)
	return nil
}

func New(conf *confengine.Config) (*Controller, error) {
	if err := setupLogger(conf); err != nil {
		return nil, err
	}

	snif, err := sniffer.New(conf)
	if err != nil {
		return nil, err
	}

	storage, err := metricstorage.New(conf)
	if err != nil {
		return nil, err
	}

	exp, err := exporter.New(conf, storage)
	if err != nil {
		return nil, err
	}

	pl, err := pipeline.New(conf)
	if err != nil {
		return nil, err
	}

	svr, err := server.New(conf)
	if err != nil {
		return nil, err
	}

	ports := make(map[socket.Port]socket.L7Proto)
	pools := make(map[socket.L7Proto]protocol.ConnPool)
	for _, pp := range snif.L7Ports() {
		ports[pp.Port] = pp.Proto
		if _, ok := pools[pp.Proto]; !ok {
			f, err := protocol.Get(pp.Proto)
			if err != nil {
				return nil, err
			}
			pools[pp.Proto] = f()
		}
	}

	var cfg Config
	if err := conf.UnpackChild("controller", &cfg); err != nil {
		return nil, err
	}
	roundtrips := make(chan socket.RoundTrip, common.Concurrency())
	ctx, cancel := context.WithCancel(context.Background())
	return &Controller{
		ctx:        ctx,
		cancel:     cancel,
		cfg:        cfg,
		pl:         pl,
		snif:       snif,
		pools:      pools,
		ports:      ports,
		svr:        svr,
		exp:        exp,
		storage:    storage,
		roundtrips: roundtrips,
	}, nil
}

func (c *Controller) decideProto(st socket.Tuple) (socket.Port, protocol.ConnPool) {
	if p, ok := c.ports[st.SrcPort]; ok {
		return st.SrcPort, c.pools[p]
	}
	if p, ok := c.ports[st.DstPort]; ok {
		return st.DstPort, c.pools[p]
	}
	return 0, nil
}

func (c *Controller) Start() error {
	c.setup()

	for i := 0; i < common.Concurrency(); i++ {
		go wait.Until(c.ctx, c.consumeRoundTrip)
	}

	if c.svr != nil {
		go c.svr.ListenAndServe()
	}

	c.snif.SetOnL4Packet(func(pkt socket.L4Packet) {
		port, pool := c.decideProto(pkt.SocketTuple())
		if pool == nil {
			return
		}
		conn := pool.GetOrCreate(pkt.SocketTuple(), port)
		if conn == nil {
			return
		}
		err := conn.OnL4Packet(pkt, c.roundtrips)
		// TODO(mando): 考虑异常断开或者没有 fin 包的情况也需要清理
		if errors.Is(err, protocol.ErrConnClosed) {
			pool.Delete(pkt.SocketTuple())
		}
	})

	return nil
}

func (c *Controller) setup() {
	if c.svr == nil {
		return
	}

	c.svr.RegisterGetRoute("/protocol/metrics", func(w http.ResponseWriter, r *http.Request) {
		if c.storage == nil {
			return
		}

		for _, pool := range c.pools {
			pool.OnStats(func(stats connstream.TupleStats) {
				c.updatePoolPromMetrics(stats)
			})
		}
		c.storage.WritePrometheus(w)
	})
}

func (c *Controller) updatePoolPromMetrics(stats connstream.TupleStats) {
	if !c.cfg.Layer4Metrics.Enabled {
		return
	}

	var lbs labels.Labels
	for _, l := range c.cfg.Layer4Metrics.RequiredLabels {
		switch l {
		case "source.host":
			lbs = append(lbs, labels.Label{Name: "src_host", Value: stats.Tuple.SrcIP.String()})
		case "source.port":
			lbs = append(lbs, labels.Label{Name: "src_port", Value: strconv.Itoa(int(stats.Tuple.SrcPort))})
		case "destination.host":
			lbs = append(lbs, labels.Label{Name: "dst_host", Value: stats.Tuple.DstIP.String()})
		case "destination.port":
			lbs = append(lbs, labels.Label{Name: "dst_port", Value: strconv.Itoa(int(stats.Tuple.DstPort))})
		}
	}

	c.storage.Update(
		metricstorage.ConstMetric{
			Model:  metricstorage.ModelCounter,
			Name:   "layer4_packets_total",
			Labels: lbs,
			Value:  float64(stats.Stats.Packets),
		},
		metricstorage.ConstMetric{
			Model:  metricstorage.ModelCounter,
			Name:   "layer4_bytes_total",
			Labels: lbs,
			Value:  float64(stats.Stats.Bytes),
		},
	)
}

// TODO(mando): 待实现 需要考虑到配置以及 sniffer 的热更新
func (c *Controller) Reload() error {
	return nil
}

func (c *Controller) Stop() {
	c.snif.Close()
	c.cancel()
}

func (c *Controller) consumeRoundTrip() {
	for {
		select {
		case rt := <-c.roundtrips:
			record := common.NewRecord(common.RecordRoundTrips, rt)
			c.exp.Export(record)
			c.pl.Range(record, func(dst *common.Record) {
				c.exp.Export(dst)
			})

		case <-c.ctx.Done():
			return
		}
	}
}
