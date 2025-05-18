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
	"time"

	"github.com/prometheus/prometheus/prompb"

	"github.com/packetd/packetd/confengine"
)

type Config struct {
	Enabled     bool          `config:"enabled"`
	Expired     time.Duration `config:"expired"`
	VmHistogram bool          `config:"vmHistogram"`
}

type Storage struct {
	cfg  Config
	set  *Set
	done chan struct{}
}

// New 创建并返回 Storage 实例
//
// 当 .Enabled 为 false 时会返回空指针 调用方需先判断
func New(conf *confengine.Config) (*Storage, error) {
	var config Config
	if err := conf.UnpackChild("metricsStorage", &config); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, nil
	}

	if config.Expired <= 0 {
		config.Expired = 5 * time.Minute
	}
	storage := &Storage{
		cfg:  config,
		set:  newSet(config.Expired),
		done: make(chan struct{}),
	}
	go storage.gc()
	return storage, nil
}

func (s *Storage) Update(cms ...ConstMetric) {
	for i := 0; i < len(cms); i++ {
		cm := cms[i]
		switch cm.Model {
		case ModelCounter:
			inst := s.set.GetOrCreateCounter(cm.Name)
			inst.Add(cm.Value, cm.Labels)

		case ModelGauge:
			inst := s.set.GetOrCreateGauge(cm.Name)
			inst.Set(cm.Value, cm.Labels)

		case ModelHistogram:
			if s.cfg.VmHistogram {
				inst := s.set.GetOrCreateVmHistogram(cm.Name)
				inst.Observe(cm.Value, cm.Labels)
				continue
			}
			inst := s.set.GetOrCreateHistogram(cm.Name, DefBuckets(cm.Unit))
			inst.Observe(cm.Value, cm.Labels)
		}
	}
}

func (s *Storage) gc() {
	ticker := time.NewTicker(s.cfg.Expired / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.set.RemoveExpired()
		case <-s.done:
			return
		}
	}
}

func (s *Storage) WritePrometheus(w io.Writer) {
	s.set.WritePrometheus(w)
}

func (s *Storage) WriteRequest() *prompb.WriteRequest {
	return s.set.WriteRequest()
}

func (s *Storage) Close() {
	close(s.done)
}
