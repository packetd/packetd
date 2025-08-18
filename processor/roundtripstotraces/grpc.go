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
	"github.com/packetd/packetd/protocol/pgrpc"
)

// https://opentelemetry.io/docs/specs/semconv/rpc/grpc/

func init() {
	register(socket.L7ProtoGRPC, newGRPCConverter())
}

type grpcConverter struct{}

func newGRPCConverter() converter {
	return &grpcConverter{}
}

func (c *grpcConverter) Proto() socket.L7Proto {
	return socket.L7ProtoGRPC
}

func (c *grpcConverter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*pgrpc.Request)
	rsp := rt.Response().(*pgrpc.Response)

	tc := extractTraceContext(req.Metadata, rsp.Metadata)

	span := ptrace.NewSpan()
	span.SetName(req.Service)
	span.SetTraceID(tc.TraceID)
	span.SetParentSpanID(tc.SpanID)
	span.SetSpanID(tracekit.RandomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	attr := span.Attributes()
	attr.PutStr("rpc.system", "grpc")
	attr.PutStr("rpc.method", "POST")
	attr.PutStr("rpc.service", req.Service)
	attr.PutStr("rpc.grpc.status_code", rsp.Status)
	attr.PutInt("rpc.request.size", int64(req.Size))
	attr.PutInt("rpc.response.size", int64(rsp.Size))

	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))

	for k, v := range req.Metadata {
		lst := attr.PutEmptySlice("rpc.grpc.request.metadata." + strings.ToLower(k))
		for _, item := range v {
			lst.AppendEmpty().SetStr(item)
		}
	}
	for k, v := range rsp.Metadata {
		lst := attr.PutEmptySlice("rpc.grpc.response.metadata." + strings.ToLower(k))
		for _, item := range v {
			lst.AppendEmpty().SetStr(item)
		}
	}

	return span
}
