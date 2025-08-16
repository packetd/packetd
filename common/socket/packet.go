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

package socket

import (
	"fmt"
	"time"

	"github.com/packetd/packetd/internal/json"
)

// RoundTrip 代表了一次网络来回
//
// 所有实现的应用层协议都应该具备 `单次请求来回` 特性
type RoundTrip interface {
	// Proto 返回 Layer7 协议
	Proto() L7Proto

	// Request 返回请求结构体 格式由实现方自行定义
	Request() any

	// Response 返回响应结构体 格式由实现方自行定义
	Response() any

	// Duration 请求耗时
	Duration() time.Duration

	// Validate 校验请求是否正确
	Validate() bool
}

func JSONMarshalRoundTrip(rt RoundTrip) ([]byte, error) {
	type R struct {
		Proto    L7Proto
		Request  any
		Response any
		Duration string
	}
	return json.Marshal(R{
		Proto:    rt.Proto(),
		Request:  rt.Request(),
		Response: rt.Response(),
		Duration: rt.Duration().String(),
	})
}

// L4Packet 表示 4 层网络数据包
//
// 应该有 TCP/UDP 两种继承实现
type L4Packet interface {
	// Proto 返回 4 层协议
	Proto() L4Proto

	// SocketTuple 返回 Socket 四元组
	SocketTuple() Tuple

	// ArrivedTime 数据包到达时间
	ArrivedTime() time.Time
}

// TCPSegment TCP L4Packet 接口实现
type TCPSegment struct {
	Tuple   Tuple
	Time    time.Time
	FIN     bool
	Seq     uint32
	Payload []byte
}

func (s TCPSegment) Proto() L4Proto {
	return L4ProtoTCP
}

func (s TCPSegment) SocketTuple() Tuple {
	return s.Tuple
}

func (s TCPSegment) ArrivedTime() time.Time {
	return s.Time
}

func (s TCPSegment) String() string {
	return fmt.Sprintf("stream %s seq: %d recv %d bytes", s.Tuple, s.Seq, len(s.Payload))
}

// UDPDatagram UDP L4Packet 接口实现
type UDPDatagram struct {
	Tuple   Tuple
	Time    time.Time
	Payload []byte
}

func (s UDPDatagram) Proto() L4Proto {
	return L4ProtoUDP
}

func (s UDPDatagram) SocketTuple() Tuple {
	return s.Tuple
}

func (s UDPDatagram) ArrivedTime() time.Time {
	return s.Time
}

func (s UDPDatagram) String() string {
	return fmt.Sprintf("stream %s recv %d bytes", s.Tuple, len(s.Payload))
}
