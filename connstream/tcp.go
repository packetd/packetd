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
	"math"
	"sync/atomic"
	"time"

	"github.com/packetd/packetd/common/socket"
)

/*
* TCP Layout
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          Source Ports          |       Destination Ports        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        Sequence Number                        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                    Acknowledgment Number                      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Data |           |U|A|P|R|S|F|                               |
| Offset| Reserved  |R|C|S|S|Y|I|            Window             |
|       |           |G|K|H|T|N|N|                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Checksum            |         Urgent Pointer        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                    Options                    |    Padding    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Data                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/

type tcpStream struct {
	st       socket.Tuple // 使用 st 作为 Stream 的唯一标识
	lastAck  uint64       // 虚拟的数据流最后一次 ack 序号
	cw       *chunkWriter // chunk 分批写入
	closed   atomic.Bool  // 链接是否结束态标识
	activeAt time.Time    // 链接最后处理数据的时间
	stats    Stats
}

// NewTCPStream 根据 socket.Tuple 创建 TCPStream 实例
func NewTCPStream(st socket.Tuple) Stream {
	stream := &tcpStream{
		st: st,
		cw: newChunkWriter(),
	}
	return stream
}

func (s *tcpStream) SocketTuple() socket.Tuple {
	return s.st
}

func (s *tcpStream) ActiveAt() time.Time {
	return s.activeAt
}

func (s *tcpStream) IsClosed() bool {
	return s.closed.Load()
}

func (s *tcpStream) Stats() Stats {
	stats := s.stats
	s.stats = Stats{}
	return stats
}

func (s *tcpStream) Write(pkt socket.L4Packet, decodeFunc DecodeFunc) error {
	seg := pkt.(*socket.TCPSegment)
	s.activeAt = time.Now()

	// 已经关闭的数据流不允许再写入
	if s.closed.Load() {
		return ErrClosed
	}
	s.stats.Packets++

	// FIN Flag 标志链接已经终止
	if seg.FIN {
		// stream 仅需要被正确关闭一次 此状态不可逆
		// 收到 FIN Flag 标识着本次请求的发送端已经没有任何东西可以发送
		// 因此将 reader 设置为 io.EOF
		if s.closed.Swap(true) {
			s.cw.Close()
		}
		// 如果是收到来自对端的 FIN 信号后 仅代表对端已经 `无数据可以发送`
		// 但此时本端可能还没有把剩余的数据全部读完 所以处理进程还是需要继续
	}

	// 无数据内容不处理
	if len(seg.Payload) == 0 {
		return nil
	}

	seq := uint64(seg.Seq)
	n := seq + uint64(len(seg.Payload))
	s.stats.Bytes += uint64(len(seg.Payload))

	// seq 已经超过了 uint32 上限 将会重头计数
	if n >= uint64(math.MaxUint32) {
		s.lastAck = 0
		n = n - math.MaxUint32
	}

	payload := seg.Payload

	// 收到了更早之前的数据包 不做处理
	// 可能是因为重传 或者是数据包阻塞在了某个网络节点上
	if s.lastAck >= n {
		return nil
	}

	switch {
	case s.lastAck > seq:
		// 数据收了一半 此时仅需写入后半部分即可
		delta := s.lastAck - seq
		payload = payload[delta:]

	case s.lastAck < seq:
		// stream 监听到的第一个 segment 已经是在一个数据流中间
		// 不做处理 直接提交给应用层
		// TCP Layer 不负责进行协议数据的切割
	}

	s.cw.Write(payload, decodeFunc)
	s.lastAck = n // 更新 lastack 代表字节流`已经`收到的最后一个序号
	return nil
}
