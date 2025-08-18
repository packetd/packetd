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

package tracekit

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceIDFromHTTPHeader(t *testing.T) {
	genTc := func(traceID, spanID string) TraceContext {
		tid, _ := trace.TraceIDFromHex(traceID)
		sid, _ := trace.SpanIDFromHex(spanID)
		return TraceContext{
			TraceID: pcommon.TraceID(tid),
			SpanID:  pcommon.SpanID(sid),
		}
	}

	tests := []struct {
		name        string
		traceParent string
		tc          TraceContext
	}{
		{
			name:        "valid",
			traceParent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			tc:          genTc("0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331"),
		},
		{
			name:        "invalid traceid",
			traceParent: "00-0af7651916cd43dd8448eb211c80319!-b7ad6b7169203331-01",
			tc:          TraceContext{},
		},
		{
			name:        "invalid version",
			traceParent: "02-0af7651916cd43dd8448eb211c80319!-b7ad6b7169203331-01",
			tc:          TraceContext{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := make(http.Header)
			header.Set(headerTraceParent, tt.traceParent)

			got, _ := TraceIDFromHTTPHeader(header)
			assert.Equal(t, tt.tc, got)
		})
	}
}
