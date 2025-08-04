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
	"github.com/packetd/packetd/protocol/pamqp"
)

// https://opentelemetry.io/docs/specs/semconv/messaging/rabbitmq/

func init() {
	register(socket.L7ProtoAMQP, newAMQPConverter())
}

type amqpConverter struct{}

func newAMQPConverter() converter {
	return &amqpConverter{}
}

func (c *amqpConverter) Proto() socket.L7Proto {
	return socket.L7ProtoAMQP
}

func (c *amqpConverter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*pamqp.Request)
	rsp := rt.Response().(*pamqp.Response)

	name := req.ClassMethod.Class + "." + req.ClassMethod.Method
	span := ptrace.NewSpan()
	span.SetName(name)
	span.SetTraceID(randomTraceID())
	span.SetSpanID(randomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	packet := &pamqp.Packet{}
	if req.Packet != nil {
		packet = req.Packet
	}

	attr := span.Attributes()
	attr.PutStr("messaging.name", "amqp")
	attr.PutStr("messaging.operation.name", name)
	attr.PutInt("messaging.message.body.size", int64(rsp.Size))
	attr.PutStr("messaging.amqp.destination.routing_key", packet.RoutingKey)
	attr.PutStr("messaging.amqp.destination.exchange_name", packet.ExchangeName)
	attr.PutStr("messaging.amqp.destination.queue_name", packet.QueueName)

	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))

	return span
}
