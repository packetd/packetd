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

package metricstorage

import (
	"io"
	"sync"
	"time"

	"github.com/prometheus/prometheus/prompb"

	"github.com/packetd/packetd/internal/fasttime"
	"github.com/packetd/packetd/internal/labels"
)

type gauge struct {
	val     float64
	lbs     labels.Labels
	updated int64
}

type Gauge struct {
	mut     sync.RWMutex
	name    string
	gauges  map[uint64]*gauge
	expired time.Duration
}

func NewGauge(name string, expired time.Duration) *Gauge {
	return &Gauge{
		name:    name,
		expired: expired,
		gauges:  make(map[uint64]*gauge),
	}
}

func (g *Gauge) Set(v float64, lbs labels.Labels) {
	hash := lbs.Hash()

	g.mut.Lock()
	defer g.mut.Unlock()

	_, ok := g.gauges[hash]
	if !ok {
		g.gauges[hash] = &gauge{lbs: lbs}
	}
	g.gauges[hash].val += v
	g.gauges[hash].updated = fasttime.UnixTimestamp()
}

func (g *Gauge) RemoveExpired() {
	g.mut.Lock()
	defer g.mut.Unlock()

	now := fasttime.UnixTimestamp()
	sec := int64(g.expired.Seconds())

	for hash, inst := range g.gauges {
		if now-inst.updated > sec {
			delete(g.gauges, hash)
		}
	}
}

func (g *Gauge) WritePrometheus(w io.Writer) {
	g.mut.RLock()
	defer g.mut.RUnlock()

	for _, inst := range g.gauges {
		WritePrometheus(w, ConstMetric{
			Name:   g.name,
			Labels: inst.lbs,
			Value:  inst.val,
		})
	}
}

func (g *Gauge) PrompbSeriess() []prompb.TimeSeries {
	g.mut.RLock()
	defer g.mut.RUnlock()

	var seriess []prompb.TimeSeries
	for _, inst := range g.gauges {
		tss := ToPrompbTimeSeries(ConstMetric{
			Name:   g.name,
			Labels: inst.lbs,
			Value:  inst.val,
		})
		seriess = append(seriess, tss...)
	}
	return seriess
}
