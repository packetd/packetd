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

package roundtripstometrics

import (
	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/mapstructure"
	"github.com/packetd/packetd/internal/metricstorage"
	"github.com/packetd/packetd/processor"
)

const Name = "roundtripstometrics"

func init() {
	processor.Register(Name, New)
}

type converter interface {
	Proto() socket.L7Proto
	Convert(rt socket.RoundTrip) []metricstorage.ConstMetric
}

var converters = map[socket.L7Proto]func(config Config) converter{}

func register(proto socket.L7Proto, converter func(config Config) converter) {
	converters[proto] = converter
}

type Factory struct {
	converters map[socket.L7Proto]converter
}

func New(conf map[string]any) (processor.Processor, error) {
	cfg := &Config{}
	if err := mapstructure.Decode(conf, cfg); err != nil {
		return nil, err
	}

	impl := make(map[socket.L7Proto]converter)
	for k, f := range converters {
		impl[k] = f(*cfg)
	}
	factory := &Factory{
		converters: impl,
	}
	return factory, nil
}

func (f *Factory) Name() string {
	return Name
}

func (f *Factory) Process(record *common.Record) (*common.Record, error) {
	rt := record.Data.(socket.RoundTrip)
	impl, ok := f.converters[rt.Proto()]
	if !ok {
		return nil, nil
	}

	data := impl.Convert(rt)
	return &common.Record{
		RecordType: common.RecordMetrics,
		Data:       &common.MetricsData{Data: data},
	}, nil
}

func (f *Factory) Clean() {}
