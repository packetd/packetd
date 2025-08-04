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

package predis

import (
	"time"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

func init() {
	protocol.Register(socket.L7ProtoRedis, NewConnPool)
}

// NewConnPool 创建 Redis 协议连接池
func NewConnPool(opts common.Options) protocol.ConnPool {
	return protocol.NewL7TCPConnPool(
		role.NewSingleMatcher,
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

// DataType 定义 RESP 多种数据类型
type DataType string

const (
	// SimpleStrings RESP 单行字符串
	//
	// "+OK\r\n"
	SimpleStrings DataType = "SimpleStrings"

	// Errors RESP 错误
	//
	// "-Error message\r\n"
	Errors DataType = "Errors"

	// Integers RESP 整数
	//
	// ":1000\r\n"
	Integers DataType = "Integers"

	// BulkStrings
	//
	// "$6\r\nfoobar\r\n"
	BulkStrings DataType = "BulkStrings"

	// Array RESP 数组
	//
	// "*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	Array DataType = "Array"
)

// Request Redis 请求
type Request struct {
	Command string
	Size    int
	Proto   string
	Host    string
	Port    uint16
	Time    time.Time
}

// Response Redis 响应
type Response struct {
	DataType string
	Size     int
	Host     string
	Proto    string
	Port     uint16
	Time     time.Time
}

var _ socket.RoundTrip = (*RoundTrip)(nil)

// RoundTrip Redis 单次请求来回
//
// 实现了 socket.RoundTrip 接口
type RoundTrip struct {
	request  *Request
	response *Response
}

func (rt RoundTrip) Proto() socket.L7Proto {
	return socket.L7ProtoRedis
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
