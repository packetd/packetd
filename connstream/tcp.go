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

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/zerocopy"
)

/*
* TCP Layout
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          Source Port (2)      |       Destination Port (2)    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        Sequence Number (4)                   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                    Acknowledgment Number (4)                  |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Data |           |U|A|P|R|S|F|                               |
|Offset | Reserved  |R|C|S|S|Y|I|            Window (2)         |
|  (4)  |   (6)     |G|K|H|T|N|N|                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Checksum (2)        |         Urgent Pointer (2)    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                    Options (var, 0-40)        |    Padding    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             Data (var)                        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/

type tcpStream struct {
	st      socket.Tuple    // 使用 st 作为 Stream 的唯一标识
	lastAck uint64          // 虚拟的数据流最后一次 ack 序号
	zb      zerocopy.Buffer // chunk 分批写入
	closed  atomic.Bool     // 链接是否结束态标识
	stats   Stats
}

// NewTCPStream 根据 socket.Tuple 创建 TCPStream 实例
func NewTCPStream(st socket.Tuple) Stream {
	stream := &tcpStream{
		st: st,
		zb: zerocopy.NewBuffer(nil),
	}
	return stream
}

func (s *tcpStream) SocketTuple() socket.Tuple {
	return s.st
}

func (s *tcpStream) IsClosed() bool {
	return s.closed.Load()
}

func (s *tcpStream) Stats() Stats {
	stats := s.stats
	s.stats = Stats{}

	stats.Proto = socket.L4ProtoTCP
	return stats
}

func (s *tcpStream) Write(pkt socket.L4Packet, decodeFunc DecodeFunc) error {
	seg := pkt.(*socket.TCPSegment)

	// 已经关闭的数据流不允许再写入
	if s.closed.Load() {
		return ErrClosed
	}
	s.stats.ReceivedPackets++

	// FIN Flag 标志链接已经终止
	if seg.FIN {
		// stream 仅需要被正确关闭一次 此状态不可逆
		// 收到 FIN Flag 标识着本次请求的发送端已经没有任何东西可以发送
		// 因此将 reader 设置为 io.EOF
		if s.closed.Swap(true) {
			s.zb.Close()
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
	s.stats.ReceivedBytes += uint64(len(seg.Payload))

	// seq 已经超过了 uint32 上限 将会重头计数
	if n >= uint64(math.MaxUint32) {
		s.lastAck = 0
		n = n - math.MaxUint32
	}

	payload := seg.Payload

	// 收到了更早之前的数据包 不做处理
	// 可能是因为重传 或者是数据包阻塞在了某个网络节点上
	if s.lastAck >= n {
		s.stats.SkippedPackets++
		return nil
	}

	switch {
	case s.lastAck > seq:
		// 数据收了一半 此时仅需写入后半部分即可
		delta := s.lastAck - seq
		payload = payload[delta:]

	case s.lastAck < seq:
	}

	s.zb.Write(payload)
	if decodeFunc != nil {
		decodeFunc(s.zb)
	}
	s.lastAck = n // 更新 lastAck 代表字节流`已经`收到的最后一个序号
	return nil
}
