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
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/prometheus/prompb"

	"github.com/packetd/packetd/internal/fasttime"
	"github.com/packetd/packetd/internal/labels"
)

type histogram struct {
	vals    []float64
	sum     float64
	count   float64
	lbs     labels.Labels
	updated int64
}

type Histogram struct {
	mut        sync.RWMutex
	name       string
	bucket     []float64
	histograms map[uint64]*histogram
	expired    time.Duration
}

func NewHistogram(name string, expired time.Duration, bucket []float64) *Histogram {
	return &Histogram{
		name:       name,
		expired:    expired,
		bucket:     append(bucket, math.Inf(+1)),
		histograms: make(map[uint64]*histogram),
	}
}

func (h *Histogram) Observe(v float64, lbs labels.Labels) {
	hash := lbs.Hash()

	h.mut.Lock()
	defer h.mut.Unlock()

	_, ok := h.histograms[hash]
	if !ok {
		h.histograms[hash] = &histogram{lbs: lbs, vals: make([]float64, len(h.bucket))}
	}

	obj := h.histograms[hash]
	for i := 0; i < len(h.bucket); i++ {
		if h.bucket[i] >= v {
			obj.vals[i]++
		}
	}
	obj.count++
	obj.sum += v
	obj.updated = fasttime.UnixTimestamp()
}

func (h *Histogram) RemoveExpired() {
	h.mut.Lock()
	defer h.mut.Unlock()

	now := fasttime.UnixTimestamp()
	sec := int64(h.expired.Seconds())

	for hash, inst := range h.histograms {
		if now-inst.updated > sec {
			delete(h.histograms, hash)
		}
	}
}

func (h *Histogram) WritePrometheus(w io.Writer) {
	h.mut.RLock()
	defer h.mut.RUnlock()

	for _, inst := range h.histograms {
		for i, bucket := range h.bucket {
			le := strconv.FormatFloat(bucket, 'f', -1, 64)
			WritePrometheus(w, ConstMetric{
				Name:   h.name + "_bucket",
				Labels: append(inst.lbs, labels.Label{Name: "le", Value: le}),
				Value:  inst.vals[i],
			})
		}

		WritePrometheus(w,
			ConstMetric{Name: h.name + "_sum", Labels: inst.lbs, Value: inst.sum},
			ConstMetric{Name: h.name + "_count", Labels: inst.lbs, Value: inst.count},
		)
	}
}

func (h *Histogram) PrompbSeriess() []prompb.TimeSeries {
	h.mut.RLock()
	defer h.mut.RUnlock()

	var seriess []prompb.TimeSeries
	for _, inst := range h.histograms {
		for i, bucket := range h.bucket {
			le := strconv.FormatFloat(bucket, 'f', -1, 64)
			tss := ToPrompbTimeSeries(ConstMetric{
				Name:   h.name + "_bucket",
				Labels: append(inst.lbs, labels.Label{Name: "le", Value: le}),
				Value:  inst.vals[i],
			})
			seriess = append(seriess, tss...)
		}

		tss := ToPrompbTimeSeries(
			ConstMetric{Name: h.name + "_sum", Labels: inst.lbs, Value: inst.sum},
			ConstMetric{Name: h.name + "_count", Labels: inst.lbs, Value: inst.count},
		)
		seriess = append(seriess, tss...)
	}
	return seriess
}
