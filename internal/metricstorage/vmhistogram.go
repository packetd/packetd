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
//
// 部分代码来自 https://github.com/VictoriaMetrics/metrics/blob/master/histogram.go

package metricstorage

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/prometheus/prompb"

	"github.com/packetd/packetd/internal/fasttime"
	"github.com/packetd/packetd/internal/labels"
)

const (
	e10Min              = -9
	e10Max              = 18
	bucketsPerDecimal   = 18
	decimalBucketsCount = e10Max - e10Min
	bucketsCount        = decimalBucketsCount * bucketsPerDecimal
)

var (
	bucketMultiplier = math.Pow(10, 1.0/bucketsPerDecimal)

	lowerBucketRange = fmt.Sprintf("0...%.3e", math.Pow10(e10Min))
	upperBucketRange = fmt.Sprintf("%.3e...+Inf", math.Pow10(e10Max))

	bucketRanges     [bucketsCount]string
	bucketRangesOnce sync.Once
)

type vmHistogram struct {
	mu             sync.Mutex
	decimalBuckets [decimalBucketsCount]*[bucketsPerDecimal]uint64
	lower          uint64
	upper          uint64
	sum            float64

	updated int64
	lbs     labels.Labels
}

func (h *vmHistogram) Update(v float64) {
	h.updated = fasttime.UnixTimestamp()
	if math.IsNaN(v) || v < 0 {
		// Skip NaNs and negative values.
		return
	}
	bucketIdx := (math.Log10(v) - e10Min) * bucketsPerDecimal
	h.mu.Lock()
	h.sum += v
	if bucketIdx < 0 {
		h.lower++
	} else if bucketIdx >= bucketsCount {
		h.upper++
	} else {
		idx := uint(bucketIdx)
		if bucketIdx == float64(idx) && idx > 0 {
			// Edge case for 10^n values, which must go to the lower bucket
			// according to Prometheus logic for `le`-based histograms.
			idx--
		}
		decimalBucketIdx := idx / bucketsPerDecimal
		offset := idx % bucketsPerDecimal
		db := h.decimalBuckets[decimalBucketIdx]
		if db == nil {
			var b [bucketsPerDecimal]uint64
			db = &b
			h.decimalBuckets[decimalBucketIdx] = db
		}
		db[offset]++
	}
	h.mu.Unlock()
}

func (h *vmHistogram) visitNonZeroBuckets(f func(vmrange string, count uint64)) {
	h.mu.Lock()
	if h.lower > 0 {
		f(lowerBucketRange, h.lower)
	}
	for decimalBucketIdx, db := range h.decimalBuckets[:] {
		if db == nil {
			continue
		}
		for offset, count := range db[:] {
			if count > 0 {
				bucketIdx := decimalBucketIdx*bucketsPerDecimal + offset
				vmrange := getVMRange(bucketIdx)
				f(vmrange, count)
			}
		}
	}
	if h.upper > 0 {
		f(upperBucketRange, h.upper)
	}
	h.mu.Unlock()
}

func (h *vmHistogram) marshalTo(name string, w io.Writer) {
	countTotal := uint64(0)
	h.visitNonZeroBuckets(func(vmrange string, count uint64) {
		WritePrometheus(w, ConstMetric{
			Name:   name + "_bucket",
			Labels: append(h.lbs, labels.Label{Name: "vmrange", Value: vmrange}),
			Value:  float64(count),
		})
		countTotal += count
	})

	if countTotal == 0 {
		return
	}

	WritePrometheus(w,
		ConstMetric{Name: name + "_sum", Labels: h.lbs, Value: h.getSum()},
		ConstMetric{Name: name + "_count", Labels: h.lbs, Value: float64(countTotal)},
	)
}

func (h *vmHistogram) getSum() float64 {
	h.mu.Lock()
	sum := h.sum
	h.mu.Unlock()
	return sum
}

func getVMRange(bucketIdx int) string {
	bucketRangesOnce.Do(initBucketRanges)
	return bucketRanges[bucketIdx]
}

func initBucketRanges() {
	v := math.Pow10(e10Min)
	start := fmt.Sprintf("%.3e", v)
	for i := 0; i < bucketsCount; i++ {
		v *= bucketMultiplier
		end := fmt.Sprintf("%.3e", v)
		bucketRanges[i] = start + "..." + end
		start = end
	}
}

type VmHistogram struct {
	mut        sync.RWMutex
	name       string
	histograms map[uint64]*vmHistogram
	expired    time.Duration
}

func NewVmHistogram(name string, expired time.Duration) *VmHistogram {
	return &VmHistogram{
		name:       name,
		expired:    expired,
		histograms: make(map[uint64]*vmHistogram),
	}
}

func (h *VmHistogram) Observe(v float64, lbs labels.Labels) {
	hash := lbs.Hash()

	h.mut.Lock()
	defer h.mut.Unlock()

	_, ok := h.histograms[hash]
	if !ok {
		h.histograms[hash] = &vmHistogram{lbs: lbs}
	}

	obj := h.histograms[hash]
	obj.Update(v)
}

func (h *VmHistogram) RemoveExpired() {
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

func (h *VmHistogram) WritePrometheus(w io.Writer) {
	h.mut.RLock()
	defer h.mut.RUnlock()

	for _, inst := range h.histograms {
		inst.marshalTo(h.name, w)
	}
}

func (h *VmHistogram) PrompbSeriess() []prompb.TimeSeries {
	h.mut.RLock()
	defer h.mut.RUnlock()

	var seriess []prompb.TimeSeries
	for _, inst := range h.histograms {
		var countTotal int
		inst.visitNonZeroBuckets(func(vmrange string, count uint64) {
			tss := ToPrompbTimeSeries(
				ConstMetric{
					Name:   h.name + "_bucket",
					Labels: append(inst.lbs, labels.Label{Name: "vmrange", Value: strconv.Itoa(int(count))}),
					Value:  float64(count),
				})
			seriess = append(seriess, tss...)
			countTotal += int(count)
		})

		if countTotal == 0 {
			continue
		}

		tss := ToPrompbTimeSeries(
			ConstMetric{Name: h.name + "_sum", Labels: inst.lbs, Value: inst.getSum()},
			ConstMetric{Name: h.name + "_count", Labels: inst.lbs, Value: float64(countTotal)},
		)
		seriess = append(seriess, tss...)
	}
	return seriess
}
