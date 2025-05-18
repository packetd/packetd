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
	"github.com/packetd/packetd/protocol/ppostgresql"
)

func init() {
	register(socket.L7ProtoPostgreSQL, newPostgreSQLConverter())
}

type postgresqlConverter struct{}

func newPostgreSQLConverter() converter {
	return &postgresqlConverter{}
}

func (c *postgresqlConverter) Proto() socket.L7Proto {
	return socket.L7ProtoPostgreSQL
}

func (c *postgresqlConverter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*ppostgresql.Request)
	rsp := rt.Response().(*ppostgresql.Response)

	var name string
	namer, ok := req.Packet.(interface{ Name() string })
	if ok {
		name = namer.Name()
	}

	span := ptrace.NewSpan()
	span.SetName(name)
	span.SetTraceID(randomTraceID())
	span.SetSpanID(randomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	attr := span.Attributes()
	attr.PutStr("db.system.name", "postgresql")
	attr.PutStr("db.operation.name", name)
	attr.PutInt("db.request.size", int64(req.Size))
	attr.PutInt("db.response.size", int64(rsp.Size))

	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))

	switch packet := req.Packet.(type) {
	case *ppostgresql.QueryPacket:
		attr.PutStr("db.query.text", packet.Statement)

	case *ppostgresql.CommandCompletePacket:
		attr.PutStr("db.query.text", packet.Command)
		attr.PutInt("db.response.returned_rows", int64(packet.Rows))

	case *ppostgresql.ErrorPacket:
		attr.PutStr("error.type", packet.Severity)
		attr.PutStr("error.code", packet.SQLStateCode)
		attr.PutStr("error.sql_state", packet.Message)

	case *ppostgresql.FlagPacket:
		attr.PutStr("db.packet.flag", packet.Flag)
	}

	return span
}
