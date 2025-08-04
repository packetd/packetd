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

package pamqp

import (
	"time"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

func init() {
	protocol.Register(socket.L7ProtoAMQP, NewConnPool)
}

const maxRecordSize = 128

// NewConnPool 创建 AMQP 协议连接池
func NewConnPool(opts common.Options) protocol.ConnPool {
	return protocol.NewL7TCPConnPool(
		func() role.Matcher {
			return role.NewFuzzyMatcher(maxRecordSize, func(req, rsp *role.Object) bool {
				reqObj := req.Obj.(*Request)
				rspObj := rsp.Obj.(*Response)

				eqCh := reqObj.ChannelID == rspObj.ChannelID
				if !eqCh {
					return false
				}

				// 如果出现身份反转则调转对象
				// 反转指 Response 先与 Client 到达 比如 Consume 场景 实际上是 Server 不断给 Client 推送请求
				if reqObj.ClassMethod.IsResponseMethod() {
					// 避免出现负数时间
					if reqObj.Time.After(rspObj.Time) {
						reqObj.Time, rspObj.Time = rspObj.Time, reqObj.Time
					}
					reqObj.ClassMethod, rspObj.ClassMethod = rspObj.ClassMethod, reqObj.ClassMethod
				}

				// 控制协议 ChannelID
				if reqObj.ChannelID == 0 {
					v, ok := classMethodPairs[reqObj.ClassMethod.Method]
					if ok && v != rspObj.ClassMethod.Method {
						return false
					}
				}
				return reqObj.ClassMethod.Class == rspObj.ClassMethod.Class
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

// Request AMQP 请求
type Request struct {
	ChannelID   uint16
	Host        string
	Port        uint16
	Proto       string
	Size        int
	Time        time.Time
	Packet      *Packet
	ClassMethod *NamedClassMethod
	FrameType   string
	ErrCode     string
}

// Response AMQP 响应
type Response struct {
	ChannelID   uint16
	Host        string
	Port        uint16
	Proto       string
	Size        int
	Time        time.Time
	Packet      *Packet
	ClassMethod *NamedClassMethod
	FrameType   string
	ErrCode     string
}

// RoundTrip AMQP 单次请求来回
//
// 实现了 socket.RoundTrip 接口
type RoundTrip struct {
	request  *Request
	response *Response
}

func (rt RoundTrip) Proto() socket.L7Proto {
	return socket.L7ProtoAMQP
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
