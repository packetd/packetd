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

package pgrpc

import (
	"net/http"
	"strings"
	"time"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/phttp2"
	"github.com/packetd/packetd/protocol/role"
)

const (
	PROTO = "gRPC"
)

func init() {
	protocol.Register(socket.L7ProtoGRPC, NewConnPool)
}

const (
	trailersGrpcMessage = "grpc-message"
	trailersGrpcStatus  = "grpc-status"
)

// NewConnPool 创建 GRPC 协议连接池
func NewConnPool() protocol.ConnPool {
	return protocol.NewL7TCPConnPool(
		func() role.Matcher {
			return role.NewListMatcher(phttp2.MaxConcurrentStreams, func(req, rsp *role.Object) bool {
				return req.Obj.(*phttp2.Request).StreamID == rsp.Obj.(*phttp2.Response).StreamID
			})
		},
		func(pair *role.Pair) socket.RoundTrip {
			return &RoundTrip{
				request:  fromHTTP2Request(pair.Request.Obj.(*phttp2.Request)),
				response: fromHTTP2Response(pair.Response.Obj.(*phttp2.Response)),
			}
		},
		func(st socket.Tuple, serverPort socket.Port) protocol.Decoder {
			return phttp2.NewDecoder(st, serverPort, phttp2.WithTrailersOpt(trailersGrpcStatus, trailersGrpcMessage))
		},
	)
}

// Request GRPC 请求
type Request struct {
	StreamID uint32
	Host     string
	Port     uint16
	Proto    string
	Service  string
	Scheme   string
	Target   string
	Metadata http.Header
	Size     int
	Time     time.Time
}

func fromHTTP2Request(req *phttp2.Request) *Request {
	return &Request{
		StreamID: req.StreamID,
		Host:     req.Host,
		Port:     req.Port,
		Proto:    PROTO,
		Service:  strings.ReplaceAll(strings.Trim(req.Path, "/"), "/", "."),
		Scheme:   req.Scheme,
		Target:   req.Authority,
		Metadata: req.Header,
		Size:     req.Size,
		Time:     req.Time,
	}
}

// Response GRPC 响应
type Response struct {
	StreamID uint32
	Host     string
	Port     uint16
	Proto    string
	Status   string
	Metadata http.Header
	Size     int
	Time     time.Time
}

func fromHTTP2Response(rsp *phttp2.Response) *Response {
	return &Response{
		StreamID: rsp.StreamID,
		Host:     rsp.Host,
		Port:     rsp.Port,
		Proto:    PROTO,
		Status:   rsp.Status,
		Metadata: rsp.Header,
		Size:     rsp.Size,
		Time:     rsp.Time,
	}
}

var _ socket.RoundTrip = (*RoundTrip)(nil)

// RoundTrip GRPC 单次请求来回
//
// 实现了 socket.RoundTrip 接口
type RoundTrip struct {
	request  *Request
	response *Response
}

func (rt RoundTrip) Proto() socket.L7Proto {
	return socket.L7ProtoGRPC
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
