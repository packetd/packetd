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

package phttp2

import (
	"net/http"
	"time"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

func init() {
	protocol.Register(socket.L7ProtoHTTP2, NewConnPool)
}

const (
	// MaxConcurrentStreams 同一 TCP 链接中最多允许的 Stream 数量
	//
	// rfc7540 https://httpwg.org/specs/rfc7540.html#SettingValues
	//
	// Indicates the maximum number of concurrent streams that the sender will allow.
	// This limit is directional: it applies to the number of streams that the sender permits
	// the receiver to create. Initially, there is no limit to this value.
	// It is recommended that this value be no smaller than 100, so as to not unnecessarily limit parallelism.
	//
	// 文档建议不小于 100
	MaxConcurrentStreams = 100
)

// NewConnPool 创建 HTTP2 协议连接池
func NewConnPool(opts common.Options) protocol.ConnPool {
	return protocol.NewL7TCPConnPool(
		func() role.Matcher {
			return role.NewListMatcher(MaxConcurrentStreams, func(req, rsp *role.Object) bool {
				return req.Obj.(*Request).StreamID == rsp.Obj.(*Response).StreamID
			})
		},
		func(pair *role.Pair) socket.RoundTrip {
			return &RoundTrip{
				request:  pair.Request.Obj.(*Request),
				response: pair.Response.Obj.(*Response),
			}
		},
		func(st socket.Tuple, serverPort socket.Port) protocol.Decoder {
			return NewDecoder(st, serverPort, opts)
		},
	)
}

// Request HTTP2 请求
type Request struct {
	StreamID  uint32
	Host      string
	Port      uint16
	Proto     string
	Path      string
	Method    string
	Scheme    string
	Authority string
	Header    http.Header
	Size      int
	Time      time.Time
}

// Response HTTP/2 响应
type Response struct {
	StreamID uint32
	Host     string
	Port     uint16
	Proto    string
	Status   string
	Header   http.Header
	Size     int
	Time     time.Time
}

// RoundTrip HTTP/2 单次请求来回
//
// 实现了 socket.RoundTrip 接口
type RoundTrip struct {
	request  *Request
	response *Response
}

func (rt RoundTrip) Proto() socket.L7Proto {
	return socket.L7ProtoHTTP2
}

func (rt RoundTrip) Request() any {
	return rt.request
}

func (rt RoundTrip) Response() any {
	return rt.response
}

func (rt RoundTrip) Duration() time.Duration {
	return rt.response.Time.Sub(rt.request.Time)
}

func (rt RoundTrip) Validate() bool {
	return rt.response.Time.After(rt.request.Time)
}
