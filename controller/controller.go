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
	"io"
	"os"
	"strconv"
	"time"

	"github.com/packetd/packetd/internal/sigs"
	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/connstream"
	"github.com/packetd/packetd/exporter"
	"github.com/packetd/packetd/internal/labels"
	"github.com/packetd/packetd/internal/metricstorage"
	"github.com/packetd/packetd/internal/pubsub"
	"github.com/packetd/packetd/internal/wait"
	"github.com/packetd/packetd/logger"
	"github.com/packetd/packetd/pipeline"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/server"
	"github.com/packetd/packetd/sniffer"
)

type Controller struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cfg        Config
	configPath string

	pl   *pipeline.Pipeline
	exp  *exporter.Exporter
	svr  *server.Server
	snif sniffer.Sniffer

	pps            *portPools
	metricsStorage *metricstorage.Storage

	rtCh  chan socket.RoundTrip
	rtBus *pubsub.PubSub
}

func setupLogger(conf *confengine.Config) error {
	var opts logger.Options
	if err := conf.UnpackChild("logger", &opts); err != nil {
		return err
	}

	if opts.Filename == "" {
		opts.Filename = "packetd.roundtrip"
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

func New(conf *confengine.Config, configPath string) (*Controller, error) {
	var cfg Config
	if err := conf.UnpackChild("controller", &cfg); err != nil {
		return nil, err
	}

	if err := setupLogger(conf); err != nil {
		return nil, err
	}

	snif, err := sniffer.New(conf)
	if err != nil {
		return nil, err
	}

	metricsStorage, err := metricstorage.New(conf)
	if err != nil {
		return nil, err
	}

	exp, err := exporter.New(conf, metricsStorage)
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

	pps, err := newPortPools(snif.L7Ports(), cfg.Decoder)
	if err != nil {
		return nil, err
	}

	rtCh := make(chan socket.RoundTrip, common.Concurrency())
	ctx, cancel := context.WithCancel(context.Background())
	return &Controller{
		ctx:            ctx,
		cancel:         cancel,
		cfg:            cfg,
		configPath:     configPath,
		pl:             pl,
		snif:           snif,
		pps:            pps,
		svr:            svr,
		exp:            exp,
		metricsStorage: metricsStorage,
		rtCh:           rtCh,
		rtBus:          pubsub.New(),
	}, nil
}

func (c *Controller) Start() error {
	c.setupServer()

	for i := 0; i < common.Concurrency(); i++ {
		go wait.Until(c.ctx, c.consumeRoundTrip)
	}
	go c.removeExpiredConn()

	if c.svr != nil {
		go func() {
			err := c.svr.ListenAndServe()
			if !errors.Is(err, io.EOF) {
				logger.Errorf("failed to start server: %v", err)
			}
		}()
	}

	if c.cfg.AutoReload && c.configPath != "" {
		go c.autoReload()
	}

	c.exp.Start()
	c.snif.SetOnL4Packet(func(pkt socket.L4Packet) {
		port, pool := c.pps.DecideProto(pkt.SocketTuple())
		if pool == nil {
			return
		}
		conn := pool.GetOrCreate(pkt.SocketTuple(), port)
		if conn == nil {
			return
		}

		err := conn.OnL4Packet(pkt, c.rtCh)
		if err == nil {
			return
		}
		if errors.Is(err, protocol.ErrConnClosed) {
			// 删除链接前先记录 Stats
			// 避免 metrics 接口还没来得及记录 conn 就已经被删除
			for _, stat := range conn.Stats() {
				c.updatePoolStats(stat)
			}
			pool.Delete(pkt.SocketTuple())
			return
		}
		logger.Debugf("failed to handle %s packet: %v", pkt.SocketTuple(), err)
	})

	return nil
}

// Reload 重载配置
//
// - 重载 sniffer，仅支持重新编译 protocols rule
func (c *Controller) Reload(conf *confengine.Config) error {
	var cfg sniffer.Config
	if err := conf.UnpackChild("sniffer", &cfg); err != nil {
		return err
	}

	if err := c.snif.Reload(&cfg); err != nil {
		return err
	}
	return c.pps.Reload(c.snif.L7Ports(), c.cfg.Decoder)
}

func (c *Controller) Stop() {
	c.snif.Close()
	c.exp.Close()
	c.cancel()
}

func (c *Controller) autoReload() {
	ticker := time.NewTimer(30 * time.Second)
	defer ticker.Stop()

	getModeTime := func() time.Time {
		info, err := os.Stat(c.configPath)
		if err != nil {
			return time.Time{}
		}
		return info.ModTime()
	}

	updated := getModeTime()

	for {
		select {
		case <-ticker.C:
			t := getModeTime()
			if t != updated {
				_ = sigs.SelfReload()
				t = updated
			}

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Controller) removeExpiredConn() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.pps.RemoveExpired(c.cfg.GetConnExpired())

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Controller) recordMetrics() {
	uptime.Set(float64(time.Now().Unix() - common.Started()))

	bi := common.GetBuildInfo()
	buildInfo.WithLabelValues(bi.Version, bi.GitHash, bi.Time).Inc()
	for _, s := range c.snif.Stats() {
		snifferReceivedPackets.WithLabelValues(s.Name).Set(float64(s.Packets))
		snifferDroppedPackets.WithLabelValues(s.Name).Set(float64(s.Drops))
	}
}

func (c *Controller) updatePoolStats(stats connstream.TupleStats) {
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

	ss := stats.Stats
	switch ss.Proto {
	case socket.L4ProtoTCP:
		c.metricsStorage.Update(
			metricstorage.NewCounterConstMetric("tcp_received_packets_total", float64(ss.ReceivedPackets), lbs),
			metricstorage.NewCounterConstMetric("tcp_received_bytes_total", float64(ss.ReceivedBytes), lbs),
			metricstorage.NewCounterConstMetric("tcp_skipped_packets_total", float64(ss.SkippedPackets), lbs),
		)

	case socket.L4ProtoUDP:
		c.metricsStorage.Update(
			metricstorage.NewCounterConstMetric("udp_received_packets_total", float64(ss.ReceivedPackets), lbs),
			metricstorage.NewCounterConstMetric("udp_received_bytes_total", float64(ss.ReceivedBytes), lbs),
		)
	}
}

func (c *Controller) updateActivePoolConns(count map[socket.L4Proto]int) {
	for k, v := range count {
		c.metricsStorage.Update(metricstorage.NewGaugeConstMetric(string(k)+"_active_conns", float64(v), nil))
	}
}

func (c *Controller) consumeRoundTrip() {
	for {
		select {
		case rt := <-c.rtCh:
			handledRoundtrips.Inc()
			record := common.NewRecord(common.RecordRoundTrips, rt)
			c.publish(record)
			c.exp.Export(record)
			c.pl.Range(record, func(dst *common.Record) {
				c.exp.Export(dst)
			})

		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Controller) publish(record *common.Record) {
	switch record.RecordType {
	case common.RecordRoundTrips:
		data, ok := record.Data.(socket.RoundTrip)
		if !ok {
			return
		}

		b, err := socket.JSONMarshalRoundTrip(data)
		if err != nil {
			return
		}
		c.rtBus.Publish(b)
	}
}
