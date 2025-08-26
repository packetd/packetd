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

package connstream

import (
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/zerocopy"
)

/*
* UDP Layout
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          Source Port (2)      |       Destination Port (2)    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Length (2)          |           Checksum (2)        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Data (var)                       |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/

type udpStream struct {
	st    socket.Tuple    // 使用 st 作为 Stream 的唯一标识
	zb    zerocopy.Buffer // chunk 分批写入
	stats Stats
}

// NewUDPStream 根据 socket.Tuple 创建 UDPStream 实例
func NewUDPStream(st socket.Tuple) Stream {
	stream := &udpStream{
		st: st,
		zb: zerocopy.NewBuffer(nil),
	}
	return stream
}

func (s *udpStream) SocketTuple() socket.Tuple {
	return s.st
}

func (s *udpStream) IsClosed() bool {
	return true
}

func (s *udpStream) Stats() Stats {
	stats := s.stats
	s.stats = Stats{}

	stats.Proto = socket.L4ProtoUDP
	return stats
}

// Write 执行 socket.L4Packet 写入操作
//
// UDP 数据包没有 FIN 标识 但是也不能在收到后就 Close 否则 roundtrip 永远不会 Match
// 因此 UDP 只靠 TTL 进行删除
func (s *udpStream) Write(pkt socket.L4Packet, decodeFunc DecodeFunc) error {
	seg, ok := pkt.(*socket.UDPDatagram)
	if !ok {
		return nil
	}
	s.stats.ReceivedPackets++

	// 无数据内容不处理
	if len(seg.Payload) == 0 {
		return nil
	}

	s.stats.ReceivedBytes += uint64(len(seg.Payload))
	payload := seg.Payload
	s.zb.Write(payload)
	if decodeFunc != nil {
		decodeFunc(s.zb)
	}
	return nil
}
