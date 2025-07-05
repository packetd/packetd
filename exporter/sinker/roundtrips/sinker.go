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

package roundtrips

import (
	"io"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/exporter"
	"github.com/packetd/packetd/internal/json"
)

func init() {
	exporter.Register(common.RecordRoundTrips, New)
}

type Sinker struct {
	wr      io.WriteCloser
	encoder json.Encoder
	cfg     *exporter.RoundTripsConfig
}

func New(conf exporter.Config) (exporter.Sinker, error) {
	cfg := &conf.RoundTrips
	cfg.Validate()

	var wr io.WriteCloser
	switch {
	case cfg.Console:
		wr = os.Stdout
	default:
		wr = &lumberjack.Logger{
			Filename:   cfg.Filename,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			LocalTime:  true,
		}
	}

	return &Sinker{
		wr:      wr,
		cfg:     cfg,
		encoder: json.NewEncoder(wr),
	}, nil
}

func (s *Sinker) Name() common.RecordType {
	return common.RecordRoundTrips
}

func (s *Sinker) Sink(data any) error {
	rt, ok := data.(socket.RoundTrip)
	if !ok {
		return nil
	}

	type R struct {
		Request  any
		Response any
		Duration string
	}
	return s.encoder.Encode(R{
		Request:  rt.Request(),
		Response: rt.Response(),
		Duration: rt.Duration().String(),
	})
}

func (s *Sinker) Close() {
	s.wr.Close()
}
