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
	"github.com/packetd/packetd/protocol/pmongodb"
)

// https://opentelemetry.io/docs/specs/semconv/database/mongodb/

func init() {
	register(socket.L7ProtoMongoDB, newMongoDBConverter())
}

type mongodbConverter struct{}

func newMongoDBConverter() converter {
	return &mongodbConverter{}
}

func (c *mongodbConverter) Proto() socket.L7Proto {
	return socket.L7ProtoMongoDB
}

func (c *mongodbConverter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*pmongodb.Request)
	rsp := rt.Response().(*pmongodb.Response)

	span := ptrace.NewSpan()
	span.SetName(req.CmdName)
	span.SetTraceID(randomTraceID())
	span.SetSpanID(randomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	attr := span.Attributes()
	attr.PutStr("db.system.name", "mongodb")
	attr.PutStr("db.query.text", req.CmdValue)
	attr.PutStr("db.operation.name", req.CmdName)
	attr.PutInt("db.request.size", int64(req.Size))
	attr.PutInt("db.response.size", int64(rsp.Size))
	attr.PutStr("db.namespace", req.Source)
	attr.PutInt("db.response.status_code", int64(rsp.Code))
	attr.PutDouble("db.response.ok", rsp.Ok)

	attr.PutStr("error.type", rsp.Message)
	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))

	return span
}
