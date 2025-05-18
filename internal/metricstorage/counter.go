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

type counter struct {
	val     float64
	lbs     labels.Labels
	updated int64
}

type Counter struct {
	mut      sync.RWMutex
	name     string
	counters map[uint64]*counter
	expired  time.Duration
}

func NewCounter(name string, expired time.Duration) *Counter {
	return &Counter{
		name:     name,
		expired:  expired,
		counters: make(map[uint64]*counter),
	}
}

func (c *Counter) Inc(lbs labels.Labels) {
	c.Add(1, lbs)
}

func (c *Counter) Add(v float64, lbs labels.Labels) {
	hash := lbs.Hash()

	c.mut.Lock()
	defer c.mut.Unlock()

	_, ok := c.counters[hash]
	if !ok {
		c.counters[hash] = &counter{lbs: lbs}
	}
	c.counters[hash].val += v
	c.counters[hash].updated = fasttime.UnixTimestamp()
}

func (c *Counter) RemoveExpired() {
	c.mut.Lock()
	defer c.mut.Unlock()

	now := fasttime.UnixTimestamp()
	sec := int64(c.expired.Seconds())

	for hash, inst := range c.counters {
		if now-inst.updated > sec {
			delete(c.counters, hash)
		}
	}
}

func (c *Counter) WritePrometheus(w io.Writer) {
	c.mut.RLock()
	defer c.mut.RUnlock()

	for _, inst := range c.counters {
		WritePrometheus(w, ConstMetric{
			Name:   c.name,
			Labels: inst.lbs,
			Value:  inst.val,
		})
	}
}

func (c *Counter) PrompbSeriess() []prompb.TimeSeries {
	c.mut.RLock()
	defer c.mut.RUnlock()

	var seriess []prompb.TimeSeries
	for _, inst := range c.counters {
		tss := ToPrompbTimeSeries(ConstMetric{
			Name:   c.name,
			Labels: inst.lbs,
			Value:  inst.val,
		})
		seriess = append(seriess, tss...)
	}
	return seriess
}
