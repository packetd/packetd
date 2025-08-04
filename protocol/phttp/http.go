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

package phttp

import (
	"net/http"
	"time"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

func init() {
	protocol.Register(socket.L7ProtoHTTP, NewConnPool)
}

// NewConnPool 创建 HTTP 协议连接池
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

// Request HTTP 请求
//
// 裁剪了 http.Request 部分字段 大部分字段语义保持一致
type Request struct {
	Host       string
	Port       uint16
	Method     string
	Header     http.Header
	Proto      string
	Path       string
	URL        string
	Scheme     string
	RemoteHost string
	Close      bool
	Size       int
	Chunked    bool
	Time       time.Time
}

// Response HTTP 响应
//
// 裁剪了 http.Response 部分字段 大部分字段语义保持一致
type Response struct {
	Host       string
	Port       uint16
	Header     http.Header
	Status     string
	StatusCode int
	Proto      string
	Close      bool
	Size       int
	Chunked    bool
	Time       time.Time
}

var _ socket.RoundTrip = (*RoundTrip)(nil)

// RoundTrip HTTP 单次请求来回
//
// 实现了 socket.RoundTrip 接口
type RoundTrip struct {
	request  *Request
	response *Response
}

func (rt RoundTrip) Proto() socket.L7Proto {
	return socket.L7ProtoHTTP
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

func fromHTTPRequest(r *http.Request) *Request {
	return &Request{
		Method:     r.Method,
		Header:     r.Header,
		Proto:      r.Proto,
		URL:        r.URL.String(),
		Path:       r.URL.Path,
		Scheme:     r.URL.Scheme,
		RemoteHost: r.Host,
		Close:      r.Close,
		Size:       int(r.ContentLength),
	}
}

func fromHTTTResponse(r *http.Response) *Response {
	return &Response{
		Header:     r.Header,
		Status:     r.Status,
		StatusCode: r.StatusCode,
		Proto:      r.Proto,
		Close:      r.Close,
	}
}
