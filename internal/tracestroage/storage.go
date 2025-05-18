// Copyright 2025 The packetd Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except spans compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to spans writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tracestroage

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/packetd/packetd/common"
)

type Storage struct {
	ctx    context.Context
	cancel context.CancelFunc

	out      chan ptrace.Traces
	in       chan ptrace.Span
	batch    int
	interval time.Duration
}

func New(batch int, interval time.Duration) *Storage {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Storage{
		ctx:      ctx,
		cancel:   cancel,
		batch:    batch,
		interval: interval,
		in:       make(chan ptrace.Span, common.Concurrency()),
		out:      make(chan ptrace.Traces, 1),
	}
	go s.pack()
	return s
}

func (s *Storage) Push(span ptrace.Span) {
	select {
	case <-s.ctx.Done():
		return
	case s.in <- span:
	}
}

func (s *Storage) Close() {
	s.cancel()
}

func (s *Storage) Pop() <-chan ptrace.Traces {
	return s.out
}

func (s *Storage) sendOut(data []ptrace.Span) {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()

	resources := resourceSpans.Resource().Attributes()
	resources.PutStr("telemetry.sdk.name", common.App)
	resources.PutStr("telemetry.sdk.version", common.Version)
	resources.PutStr("telemetry.sdk.language", "golang")

	spans := resourceSpans.ScopeSpans().AppendEmpty().Spans()
	for i := 0; i < len(data); i++ {
		span := spans.AppendEmpty()
		data[i].CopyTo(span)
	}
	s.out <- traces
}

func (s *Storage) pack() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	data := make([]ptrace.Span, 0, s.batch)
	for {
		select {
		case <-s.ctx.Done():
			return

		case span := <-s.in:
			data = append(data, span)
			if len(data) >= s.batch {
				s.sendOut(data)
				data = make([]ptrace.Span, 0, s.batch)
			}

		case <-ticker.C:
			if len(data) > 0 {
				s.sendOut(data)
				data = make([]ptrace.Span, 0, s.batch)
			}
		}
	}
}
