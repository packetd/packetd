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
	"bytes"
	"time"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/splitio"
	"github.com/packetd/packetd/internal/zerocopy"
)

func newError(format string, args ...any) error {
	format = "layer4/stream: " + format
	return errors.Errorf(format, args...)
}

var (
	// ErrSocketNotMatch socket 无法正确匹配
	ErrSocketNotMatch = newError("socket not match")

	// ErrNotConfirm stream 未能正确创建
	ErrNotConfirm = newError("stream not confirm")

	// ErrClosed stream 已经处于 Close 状态
	ErrClosed = newError("closed")
)

// Stats Layer4 的统计数据
type Stats struct {
	Packets uint64
	Bytes   uint64
}

// DecodeFunc 字节流的解析方法
//
// 实现方必须保证 Layer4 的协议能够按序解析 且不依赖完整的 Payload
// Stream 使用了 zerocopy.Buffer 会对一个完整的 socket.L4Packet 进行切片写入以降低处理过程中的内存开销
// 实现方不允许修改字节流的任何内存 所有字节应该是 Readonly
type DecodeFunc func(r zerocopy.Reader)

// Stream 代表了 Layer4 通信的 1 条带方向的数据流
//
// 程序并无真实持有 `链接` 以及 FD 仅是通过网卡数据分析
// 并构造出虚拟的字节流
//
// 因此对于单个 Connection 应该有 2 条 Stream
//
// 单个 Stream 的数据读写应该是串行的 `不允许也不应该成为并发操作`
type Stream interface {
	// SocketTuple 返回 Stream socket.Tuple 标识
	SocketTuple() socket.Tuple

	// ActiveAt 返回链接最后活跃时间
	//
	// 每次 decode 前会先记录
	ActiveAt() time.Time

	// IsClosed 返回 Stream 是否已经处于结束态
	//
	// * 对于 TPC 协议 依赖 Fin Flags 或者 RST 数据包来判断
	// * 对于 UDP 协议 每次接收完数据包后均是 closed 状态
	IsClosed() bool

	// Stats 返回 Stream 打点数据
	Stats() Stats

	// Write 执行 segments 写入操作
	// 允许传入 DecodeFunc 对 Payload 进行流式解析
	//
	// Write 没有实现完整的 Layer4 协议栈 无法保证数据的完整性
	// * 对于 TCP 协议 如果假定发送方的传包顺序 pkt1 > pkt2 > pkt3
	//   而接收方收到的顺序为 pkt1 > pkt3 > pkt2 则 pkt2 就会被丢弃
	// * 对于 UDP 协议 那就没这个需求了 本身也不会没有重传机制
	Write(seg socket.L4Packet, decodeFunc DecodeFunc) error
}

// CreateStreamFunc 定义了创建 Stream 的方法
type CreateStreamFunc func(st socket.Tuple) Stream

// pipe 将两条 Stream 封装起来成一条管道
//
// l,r 并无实际顺序意义 使用两个变量来代替 Map 效率会高些
type pipe struct {
	createStream CreateStreamFunc
	l, r         Stream
}

// confirm 确认链接是否有效 遵循先左后右原则
//
// 其上层要保证 socket.Tuple 一定是成对出现的
func (p *pipe) confirm(st socket.Tuple) Stream {
	// 优先检查已经存在的 st
	if p.l != nil && p.l.SocketTuple() == st {
		return p.l
	}
	if p.r != nil && p.r.SocketTuple() == st {
		return p.r
	}

	// 从左往右创建 Stream
	// l, r 无实际方向意义 仅起标识作用
	if p.l == nil {
		p.l = p.createStream(st)
		return p.l
	}
	if p.r == nil {
		p.r = p.createStream(st)
		return p.r
	}
	return nil
}

// isClosed 返回 pipe 所持有 Stream 是否已经关闭
//
// 当且仅当两条 Stream 均处于关闭状态时才返回 true
func (p *pipe) isClosed() bool {
	if p.l != nil && !p.l.IsClosed() {
		return false
	}
	if p.r != nil && !p.r.IsClosed() {
		return false
	}
	return true
}

// Conn 代表着 1 条真正的 Layer4 链接 包含两个方向的 Stream
//
// Layer4 Connection
// | ------------------------------------------------ |
// | StreamL                                          |
// | |                                       | Decode |
// | |                              op:Write & [hook] |
// | ==============================================>  |
// |               / zerocopy.buffer /                |
// | <==============================================  |
// | op:Write & [hook]                             |  |
// |          | Decode                             |  |
// |                                          StreamR |
// | ------------------------------------------------ |
//
// 对于链接中的两条 Stream 其状态应该是一致的 即要么都可用 要么都不可用
// Conn 允许在每次执行 Stream Write 操作时挂载 DecodeFunc 保证对数据的流式处理
//
// l, r 记录这条 Conn 仅能通过两个 socket.Tuple
type Conn struct {
	pipe *pipe
	l, r socket.Tuple
}

// NewConn 创建 Layer4 Connection
//
// 传入的 tuple 仅为标识链接的两端的 socket.Tuple
func NewConn(st socket.Tuple, f CreateStreamFunc) *Conn {
	return &Conn{
		pipe: &pipe{createStream: f},
		l:    st,
		r:    st.Mirror(),
	}
}

// Stream 返回 st 所关联的 Stream
func (c *Conn) Stream(st socket.Tuple) Stream {
	return c.pipe.confirm(st)
}

type TupleStats struct {
	Tuple socket.Tuple
	Stats Stats
}

// Stats 返回 Conn 统计数据
func (c *Conn) Stats() []TupleStats {
	ts := make([]TupleStats, 0, 2)
	if c.pipe.l != nil {
		ts = append(ts, TupleStats{
			Tuple: c.l,
			Stats: c.pipe.l.Stats(),
		})
	}
	if c.pipe.r != nil {
		ts = append(ts, TupleStats{
			Tuple: c.r,
			Stats: c.pipe.r.Stats(),
		})
	}
	return ts
}

// Write 执行 socket.L4Packet 写入操作
//
// 对于 Conn 来讲 会先确定具体的 Stream 然后再调用其 Write 方法
// 返回是否写入成功
func (c *Conn) Write(seg socket.L4Packet, decodeFunc DecodeFunc) error {
	// 拒绝写入非此链接运行的 SocketTuple
	if c.l != seg.SocketTuple() && c.r != seg.SocketTuple() {
		return ErrSocketNotMatch
	}

	stream := c.pipe.confirm(seg.SocketTuple())
	if stream == nil {
		return ErrNotConfirm // 理论上不应出现
	}

	// 写入并解码数据
	return stream.Write(seg, decodeFunc)
}

// IsClosed 返回 Conn 是否已经处于结束态
func (c *Conn) IsClosed() bool {
	return c.pipe.isClosed()
}

// chunkWriter 负责将 Reader 数据切成若干 chunk 并写入 Stream
//
// 每次写入均要调用 DecodeFunc 进行数据解析
type chunkWriter struct {
	zb zerocopy.Buffer
}

func newChunkWriter() *chunkWriter {
	return &chunkWriter{
		zb: zerocopy.NewBuffer(nil),
	}
}

// writeChunk 分批写入 rb 并执行 DecodeFunc
//
// 为了保证 `每一字节` 均要被 Decode 则要求 ReadSize >= WriteSize
// 切割 chunk 的时候要考虑保证数据的连续性 部分协议会使用 CRLF 作为结尾
// 所以不要将 CR LR 分批发送
func (cw *chunkWriter) Write(payload []byte, f DecodeFunc) {
	const buffered = 64
	var l, r int
	size := len(payload)
	for {
		r += common.ReadWriteBlockSize - buffered
		if r >= size {
			r = size
			if l == r {
				return
			}
			cw.zb.Write(payload[l:r])
			if f != nil {
				f(cw.zb)
			}
			return
		}

		end := r + buffered
		if end > size {
			end = size
		}
		idx := bytes.IndexByte(payload[r:end], splitio.CharLF[0])
		if idx >= 0 {
			r += idx + 1
		}
		cw.zb.Write(payload[l:r])
		if f != nil {
			f(cw.zb)
		}
		l += r - l
	}
}

// Close 关闭 Writer 将 buffer 置为 EOF 状态
func (cw *chunkWriter) Close() {
	cw.zb.Close()
}
