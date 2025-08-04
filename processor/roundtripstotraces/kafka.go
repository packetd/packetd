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
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol/pkafka"
)

// https://opentelemetry.io/docs/specs/semconv/messaging/kafka/

func init() {
	register(socket.L7ProtoKafka, newKafkaConverter())
}

type kafkaConverter struct{}

func newKafkaConverter() converter {
	return &kafkaConverter{}
}

func (c *kafkaConverter) Proto() socket.L7Proto {
	return socket.L7ProtoKafka
}

func (c *kafkaConverter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*pkafka.Request)
	rsp := rt.Response().(*pkafka.Response)

	packet := req.Packet

	span := ptrace.NewSpan()
	span.SetName(packet.API)
	span.SetTraceID(randomTraceID())
	span.SetSpanID(randomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	attr := span.Attributes()
	attr.PutStr("messaging.name", "kafka")
	attr.PutStr("messaging.operation.name", packet.API)
	attr.PutStr("messaging.operation.version", strconv.Itoa(int(packet.APIVersion)))
	attr.PutStr("messaging.client.id", packet.ClientID)
	attr.PutStr("messaging.consumer.group.name", packet.GroupID)
	attr.PutInt("messaging.message.body.size", int64(rsp.Size))

	attr.PutStr("error.type", rsp.ErrorCode)
	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))

	return span
}
