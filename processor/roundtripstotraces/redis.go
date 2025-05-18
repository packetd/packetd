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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol/predis"
)

func init() {
	register(socket.L7ProtoRedis, newRedisConverter())
}

type redisConverter struct{}

func newRedisConverter() converter {
	return &redisConverter{}
}

func (c *redisConverter) Proto() socket.L7Proto {
	return socket.L7ProtoRedis
}

func (c *redisConverter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*predis.Request)
	rsp := rt.Response().(*predis.Response)

	span := ptrace.NewSpan()
	span.SetName(req.Command)
	span.SetTraceID(randomTraceID())
	span.SetSpanID(randomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	attr := span.Attributes()
	attr.PutStr("db.system.name", "redis")
	attr.PutStr("db.operation.name", req.Command)
	attr.PutInt("db.request.size", int64(req.Size))
	attr.PutInt("db.response.size", int64(rsp.Size))

	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))
	attr.PutStr("response.data_type", rsp.DataType)
	return span
}
