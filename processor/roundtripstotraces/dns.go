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
	"github.com/packetd/packetd/protocol/pdns"
)

func init() {
	register(socket.L7ProtoDNS, newDNSConverter())
}

type dnsConverter struct{}

func newDNSConverter() converter {
	return &dnsConverter{}
}

func (c *dnsConverter) Proto() socket.L7Proto {
	return socket.L7ProtoDNS
}

func (c *dnsConverter) Convert(rt socket.RoundTrip) ptrace.Span {
	req := rt.Request().(*pdns.Request)
	rsp := rt.Response().(*pdns.Response)

	span := ptrace.NewSpan()
	span.SetName(req.Message.QuestionSec.Name)
	span.SetTraceID(randomTraceID())
	span.SetSpanID(randomSpanID())
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(req.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(rsp.Time))

	attr := span.Attributes()
	attr.PutInt("dns.request.size", int64(req.Size))
	attr.PutInt("dns.response.size", int64(rsp.Size))
	attr.PutStr("dns.question.type", req.Message.QuestionSec.Type)

	attr.PutStr("server.address", rsp.Host)
	attr.PutInt("server.port", int64(rsp.Port))
	attr.PutStr("network.peer.address", req.Host)
	attr.PutInt("network.peer.port", int64(req.Port))

	return span
}
