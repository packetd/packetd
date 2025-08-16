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

package roundtripstotraces

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/tracekit"
	"github.com/packetd/packetd/protocol/phttp2"
)

// https://opentelemetry.io/docs/specs/semconv/registry/attributes/http/

func init() {
	register(socket.L7ProtoHTTP2, newHTTP2Converter())
}

type http2Converter struct{}

func newHTTP2Converter() converter {
	return &http2Converter{}
}

func (c *http2Converter) Proto() socket.L7Proto {
	return socket.L7ProtoHTTP2
}

func (c *http2Converter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*phttp2.Request)
	rsp := rt.Response().(*phttp2.Response)

	traceID := extractTraceID(req.Header, rsp.Header)

	span := ptrace.NewSpan()
	span.SetName(req.Method)
	span.SetTraceID(traceID)
	span.SetSpanID(tracekit.RandomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	attr := span.Attributes()
	attr.PutInt("http.request.size", int64(req.Size))
	attr.PutInt("http.response.size", int64(rsp.Size))
	attr.PutStr("http.request.method", req.Method)
	attr.PutStr("http.response.status_code", rsp.Status)

	attr.PutStr("url.full", req.Path)
	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))
	attr.PutStr("network.transport", "tcp")
	attr.PutStr("network.protocol.name", "http2")
	attr.PutStr("network.protocol.version", "2.0")

	for k, v := range req.Header {
		lst := attr.PutEmptySlice("http.request.header." + strings.ToLower(k))
		for _, item := range v {
			lst.AppendEmpty().SetStr(item)
		}
	}
	for k, v := range rsp.Header {
		lst := attr.PutEmptySlice("http.response.header." + strings.ToLower(k))
		for _, item := range v {
			lst.AppendEmpty().SetStr(item)
		}
	}

	return span
}
