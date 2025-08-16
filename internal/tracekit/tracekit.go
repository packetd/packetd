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
	"crypto/rand"
	"net/http"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/otel/trace"
)

const (
	headerTraceParent = "traceparent"
)

// TraceIDFromHTTPHeader 从 HTTP header 中提取 TraceID
//
// 格式样例
// traceparent: 00-{trace-id}-{parent-id}-{trace-flags}
func TraceIDFromHTTPHeader(h http.Header) (pcommon.TraceID, bool) {
	var empty pcommon.TraceID
	s := h.Get(headerTraceParent)
	if s == "" {
		return empty, false
	}

	parts := strings.Split(s, "-")
	if len(parts) != 4 {
		return empty, false
	}

	// 版本校验
	if parts[0] != "00" {
		return empty, false
	}

	traceID, err := trace.TraceIDFromHex(parts[1])
	if err != nil {
		return empty, false
	}
	return pcommon.TraceID(traceID), true
}

// RandomTraceID 随机生成 TraceID
func RandomTraceID() pcommon.TraceID {
	b := make([]byte, 16)
	rand.Read(b)

	ret := [16]byte{}
	for i := 0; i < 16; i++ {
		ret[i] = b[i]
	}
	return ret
}

// RandomSpanID 随机生成 SpanID
func RandomSpanID() pcommon.SpanID {
	b := make([]byte, 8)
	rand.Read(b)

	ret := [8]byte{}
	for i := 0; i < 8; i++ {
		ret[i] = b[i]
	}
	return ret
}
