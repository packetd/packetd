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

package exporter

import (
	"context"
	"time"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/internal/metricstorage"
	"github.com/packetd/packetd/internal/tracestroage"
	"github.com/packetd/packetd/logger"
)

type Exporter struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   Config

	metricsStorage *metricstorage.Storage
	tracesStorage  *tracestroage.Storage

	metricsSinker    Sinker
	tracesSinker     Sinker
	roundTripsSinker Sinker
}

func New(conf *confengine.Config, metricsStorage *metricstorage.Storage) (*Exporter, error) {
	var cfg Config
	if err := conf.UnpackChild("exporter", &cfg); err != nil {
		return nil, err
	}

	var err error
	var metricsSinker Sinker
	if cfg.Metrics.Enabled {
		f := Get(common.RecordMetrics)
		if metricsSinker, err = f(cfg); err != nil {
			return nil, err
		}
	}

	var tracesSinker Sinker
	if cfg.Traces.Enabled {
		f := Get(common.RecordTraces)
		if tracesSinker, err = f(cfg); err != nil {
			return nil, err
		}
	}

	var roundTripsSinker Sinker
	if cfg.RoundTrips.Enabled {
		f := Get(common.RecordRoundTrips)
		if roundTripsSinker, err = f(cfg); err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	exp := &Exporter{
		ctx:              ctx,
		cancel:           cancel,
		conf:             cfg,
		metricsStorage:   metricsStorage,
		tracesStorage:    tracestroage.New(cfg.Traces.Batch, cfg.Traces.Interval),
		metricsSinker:    metricsSinker,
		tracesSinker:     tracesSinker,
		roundTripsSinker: roundTripsSinker,
	}
	return exp, nil
}

func (e *Exporter) Start() {
	if e.conf.Traces.Enabled {
		go e.loopExportTraces()
	}
	if e.conf.Metrics.Enabled {
		go e.loopExportMetrics()
	}
}

func (e *Exporter) Close() {
	e.cancel()

	if e.conf.Metrics.Enabled {
		e.metricsSinker.Close()
	}
	if e.conf.Traces.Enabled {
		e.tracesSinker.Close()
	}
	if e.conf.RoundTrips.Enabled {
		e.roundTripsSinker.Close()
	}

	e.metricsStorage.Close()
}

func (e *Exporter) Export(record *common.Record) {
	switch record.RecordType {
	case common.RecordMetrics:
		if e.metricsStorage == nil {
			return
		}

		data, ok := record.Data.(*common.MetricsData)
		if !ok {
			return
		}
		e.metricsStorage.Update(data.Data...)

	case common.RecordTraces:
		if !e.conf.Traces.Enabled {
			return
		}

		data, ok := record.Data.(*common.TracesData)
		if !ok {
			return
		}
		e.tracesStorage.Push(data.Data)

	case common.RecordRoundTrips:
		if !e.conf.RoundTrips.Enabled {
			return
		}

		data, ok := record.Data.(socket.RoundTrip)
		if !ok {
			return
		}
		e.roundTripsSinker.Sink(data)
	}
}

func (e *Exporter) loopExportMetrics() {
	if !e.conf.Metrics.Enabled {
		return
	}

	ticker := time.NewTicker(e.conf.Metrics.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return

		case <-ticker.C:
			if err := e.metricsSinker.Sink(e.metricsStorage.WriteRequest()); err != nil {
				logger.Errorf("sink metrics failed: %v", err)
			}
		}
	}
}

func (e *Exporter) loopExportTraces() {
	if !e.conf.Traces.Enabled {
		return
	}

	for {
		select {
		case <-e.ctx.Done():
			return

		case traces := <-e.tracesStorage.Pop():
			if err := e.tracesSinker.Sink(traces); err != nil {
				logger.Errorf("sink traces failed: %v", err)
			}
		}
	}
}
