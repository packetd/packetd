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

package pmongodb

import (
	"time"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

func init() {
	protocol.Register(socket.L7ProtoMongoDB, NewConnPool)
}

const maxRecordSize = 64

// NewConnPool 创建 MongoDB 协议连接池
func NewConnPool(opts common.Options) protocol.ConnPool {
	return protocol.NewL7TCPConnPool(
		func() role.Matcher {
			return role.NewListMatcher(maxRecordSize, func(req, rsp *role.Object) bool {
				return req.Obj.(*Request).ID == rsp.Obj.(*Response).ID
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

// Request MongoDB 请求
type Request struct {
	ID         int32
	Host       string
	Port       uint16
	Proto      string
	OpCode     string
	Source     string
	Collection string
	CmdName    string
	CmdValue   string
	Size       int
	Time       time.Time
}

// Response MongoDB 响应
type Response struct {
	ID      int32
	Host    string
	Port    uint16
	Proto   string
	OpCode  string
	Ok      float64
	Code    int32
	Message string
	Size    int
	Time    time.Time
}

var _ socket.RoundTrip = (*RoundTrip)(nil)

// RoundTrip MongoDB 单次请求来回
//
// 实现了 socket.RoundTrip 接口
type RoundTrip struct {
	request  *Request
	response *Response
}

func (rt RoundTrip) Proto() socket.L7Proto {
	return socket.L7ProtoMongoDB
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
