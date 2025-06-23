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
	"time"

	"github.com/packetd/packetd/common/socket"
)

/*
* UDP Layout
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          Source Ports          |       Destination Ports        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Length              |           Checksum            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Data                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/

type udpStream struct {
	st       socket.Tuple // 使用 st 作为 Stream 的唯一标识
	cw       *chunkWriter // chunk 分批写入
	activeAt time.Time    // 链接最后处理数据的时间
	stats    Stats
}

// NewUDPStream 根据 socket.Tuple 创建 UDPStream 实例
func NewUDPStream(st socket.Tuple) Stream {
	stream := &udpStream{
		st: st,
		cw: newChunkWriter(),
	}
	return stream
}

func (s *udpStream) SocketTuple() socket.Tuple {
	return s.st
}

func (s *udpStream) ActiveAt() time.Time {
	return s.activeAt
}

func (s *udpStream) IsClosed() bool {
	return true
}

func (s *udpStream) Stats() Stats {
	stats := s.stats
	s.stats = Stats{}
	return stats
}

// Write 执行 socket.L4Packet 写入操作
//
// UDP 数据包每次写入均返回链接关闭 不用等待
func (s *udpStream) Write(pkt socket.L4Packet, decodeFunc DecodeFunc) error {
	seg := pkt.(*socket.UDPDatagram)
	s.activeAt = time.Now()
	s.stats.Packets++

	// 无数据内容不处理
	if len(seg.Payload) == 0 {
		return ErrClosed
	}

	s.stats.Bytes += uint64(len(seg.Payload))
	payload := seg.Payload
	s.cw.Write(payload, decodeFunc)
	return ErrClosed
}
