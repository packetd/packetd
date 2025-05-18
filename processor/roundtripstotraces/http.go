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
	"github.com/packetd/packetd/protocol/phttp"
)

func init() {
	register(socket.L7ProtoHTTP, newHTTPConverter())
}

type httpConverter struct{}

func newHTTPConverter() converter {
	return &httpConverter{}
}

func (c *httpConverter) Proto() socket.L7Proto {
	return socket.L7ProtoHTTP
}

func (c *httpConverter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*phttp.Request)
	rsp := rt.Response().(*phttp.Response)

	span := ptrace.NewSpan()
	span.SetName(req.Method)
	span.SetTraceID(randomTraceID())
	span.SetSpanID(randomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	attr := span.Attributes()
	attr.PutInt("http.request.size", int64(req.Size))
	attr.PutInt("http.response.size", int64(rsp.Size))
	attr.PutStr("http.request.method", req.Method)
	attr.PutInt("http.response.status_code", int64(rsp.StatusCode))

	attr.PutStr("url.full", req.URL)
	attr.PutStr("url.scheme", req.Scheme)
	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))
	attr.PutStr("network.transport", "tcp")
	attr.PutStr("network.protocol.name", "http")
	attr.PutStr("network.protocol.version", "1.0")

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
