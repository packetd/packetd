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
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/prometheus/prometheus/prompb"

	"github.com/packetd/packetd/internal/fasttime"
	"github.com/packetd/packetd/internal/labels"
)

type Unit uint8

const (
	UnitBytes Unit = iota
	UnitSeconds
)

func KB(n int) float64 {
	return float64(n) * 1024
}

func MB(n int) float64 {
	return float64(n) * 1024 * 1024
}

var (
	// DefSizeDistribution 默认的数据量桶分布
	DefSizeDistribution = []float64{
		KB(10), KB(100), KB(250), KB(500),
		MB(1), MB(5), MB(10), MB(20), MB(30), MB(50),
		MB(80), MB(100), MB(150), MB(200), MB(500),
	}

	// DefObserveDuration 默认的时间桶分布
	DefObserveDuration = []float64{
		0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 60, 120, 300, 600,
	}
)

func DefBuckets(u Unit) []float64 {
	switch u {
	case UnitBytes:
		return DefSizeDistribution
	case UnitSeconds:
		return DefObserveDuration
	}
	return nil
}

type Model uint8

const (
	ModelCounter Model = iota
	ModelGauge
	ModelHistogram
)

type ConstMetric struct {
	Unit   Unit
	Model  Model
	Name   string
	Labels labels.Labels
	Value  float64
}

type Set struct {
	mut          sync.RWMutex
	expired      time.Duration
	counters     map[string]*Counter
	histograms   map[string]*Histogram
	vmHistograms map[string]*VmHistogram
	gauges       map[string]*Gauge
}

func newSet(expired time.Duration) *Set {
	return &Set{
		expired:      expired,
		counters:     make(map[string]*Counter),
		histograms:   make(map[string]*Histogram),
		vmHistograms: make(map[string]*VmHistogram),
		gauges:       make(map[string]*Gauge),
	}
}

func (s *Set) Reset() {
	s.mut.Lock()
	defer s.mut.Unlock()

	s.counters = make(map[string]*Counter)
	s.histograms = make(map[string]*Histogram)
	s.vmHistograms = make(map[string]*VmHistogram)
	s.gauges = make(map[string]*Gauge)
}

func (s *Set) GetOrCreateCounter(name string) *Counter {
	s.mut.RLock()
	inst, ok := s.counters[name]
	if ok {
		s.mut.RUnlock()
		return inst
	}
	s.mut.RUnlock()

	s.mut.Lock()
	defer s.mut.Unlock()

	if inst, ok = s.counters[name]; ok {
		return inst
	}
	s.counters[name] = NewCounter(name, s.expired)
	return s.counters[name]
}

func (s *Set) GetOrCreateGauge(name string) *Gauge {
	s.mut.RLock()
	inst, ok := s.gauges[name]
	if ok {
		s.mut.RUnlock()
		return inst
	}
	s.mut.RUnlock()

	s.mut.Lock()
	defer s.mut.Unlock()

	if inst, ok = s.gauges[name]; ok {
		return inst
	}
	s.gauges[name] = NewGauge(name, s.expired)
	return s.gauges[name]
}

func (s *Set) GetOrCreateHistogram(name string, buckets []float64) *Histogram {
	s.mut.RLock()
	inst, ok := s.histograms[name]
	if ok {
		s.mut.RUnlock()
		return inst
	}
	s.mut.RUnlock()

	s.mut.Lock()
	defer s.mut.Unlock()

	if inst, ok = s.histograms[name]; ok {
		return inst
	}
	s.histograms[name] = NewHistogram(name, s.expired, buckets)
	return s.histograms[name]
}

func (s *Set) GetOrCreateVmHistogram(name string) *VmHistogram {
	s.mut.RLock()
	inst, ok := s.vmHistograms[name]
	if ok {
		s.mut.RUnlock()
		return inst
	}
	s.mut.RUnlock()

	s.mut.Lock()
	defer s.mut.Unlock()

	if inst, ok = s.vmHistograms[name]; ok {
		return inst
	}
	s.vmHistograms[name] = NewVmHistogram(name, s.expired)
	return s.vmHistograms[name]
}

func (s *Set) WritePrometheus(w io.Writer) {
	s.mut.RLock()
	defer s.mut.RUnlock()

	for _, inst := range s.counters {
		inst.WritePrometheus(w)
	}
	for _, inst := range s.gauges {
		inst.WritePrometheus(w)
	}
	for _, inst := range s.histograms {
		inst.WritePrometheus(w)
	}
	for _, inst := range s.vmHistograms {
		inst.WritePrometheus(w)
	}
}

func (s *Set) RemoveExpired() {
	s.mut.RLock()
	defer s.mut.RUnlock()

	for _, inst := range s.counters {
		inst.RemoveExpired()
	}
	for _, inst := range s.gauges {
		inst.RemoveExpired()
	}
	for _, inst := range s.histograms {
		inst.RemoveExpired()
	}
	for _, inst := range s.vmHistograms {
		inst.RemoveExpired()
	}
}

func (s *Set) WriteRequest() *prompb.WriteRequest {
	s.mut.RLock()
	defer s.mut.RUnlock()

	var seriess []prompb.TimeSeries
	for _, inst := range s.counters {
		seriess = append(seriess, inst.PrompbSeriess()...)
	}
	for _, inst := range s.gauges {
		seriess = append(seriess, inst.PrompbSeriess()...)
	}
	for _, inst := range s.histograms {
		seriess = append(seriess, inst.PrompbSeriess()...)
	}
	for _, inst := range s.vmHistograms {
		seriess = append(seriess, inst.PrompbSeriess()...)
	}
	return &prompb.WriteRequest{
		Timeseries: seriess,
	}
}

func WritePrometheus(w io.Writer, metrics ...ConstMetric) {
	for i := 0; i < len(metrics); i++ {
		metric := metrics[i]
		w.Write([]byte(metric.Name))
		w.Write([]byte(`{`))

		var n int
		for _, label := range metric.Labels {
			if n > 0 {
				w.Write([]byte(`,`))
			}
			n++
			w.Write([]byte(label.Name))
			w.Write([]byte(`="`))
			w.Write([]byte(label.Value))
			w.Write([]byte(`"`))
		}

		w.Write([]byte("} "))
		w.Write([]byte(fmt.Sprintf("%f", metric.Value)))
		w.Write([]byte("\n"))
	}
}

func ToPrompbTimeSeries(metrics ...ConstMetric) []prompb.TimeSeries {
	ts := fasttime.UnixTimestamp() * 1000
	seriess := make([]prompb.TimeSeries, 0, len(metrics))
	for _, metric := range metrics {
		lbs := make([]prompb.Label, 0, len(metric.Labels)+1)
		lbs = append(lbs, prompb.Label{
			Name:  "__name__",
			Value: metric.Name,
		})
		for _, label := range metric.Labels {
			lbs = append(lbs, prompb.Label{
				Name:  label.Name,
				Value: label.Value,
			})
		}
		seriess = append(seriess, prompb.TimeSeries{
			Labels: lbs,
			Samples: []prompb.Sample{{
				Value:     metric.Value,
				Timestamp: ts,
			}},
		})
	}
	return seriess
}
