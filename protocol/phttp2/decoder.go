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

package phttp2

import (
	"bytes"
	"encoding/binary"
	"math"
	"time"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/bufpool"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

func newError(format string, args ...any) error {
	format = "http2/decoder: " + format
	return errors.Errorf(format, args...)
}

var (
	errInvalidBytes    = newError("invalid bytes")
	errDecodeHeader    = newError("decode Header failed")
	errSkipConnPreface = newError("skip connection preface")
)

var connPreface = []byte("HTTP/2.0\r\n\r\nSM\r\n\r\n")

const (
	// maxPayloadSize HTTP2 帧最大 payload 大小
	maxPayloadSize = 0xFFFFFF

	// headerMask HTTP2 header 掩码
	headerMask = 0x7fffffff
)

// state 记录着 decoder 的处理状态
type state int

const (
	// stateDecodeHeader 处于解析 header 状态
	stateDecodeHeader state = iota

	// stateDecodePayload 处于解析 Payload 状态
	stateDecodePayload
)

// decoder HTTP/2 协议解析器
//
// decoder 利用支持单链接多 Stream 的协议数据解析 内部维护了 Stream 池
// decoder 负责正确地将数据包拆分并提交至 streamDecoder 做进一步解析
type decoder struct {
	t0         time.Time
	st         socket.TupleRaw
	serverPort socket.Port
	opt        *option

	rbuf    *bytes.Buffer
	hfd     *HeaderFieldDecoder
	streams map[uint32]*streamDecoder

	prevData    *streamData // 上一轮解析的状态
	tail        []byte      // 尾部数据拼接 仅允许拼接一次 避免上一轮切割了部分数据
	partial     uint8       // 标记上一轮的 header 是否待拼接
	maxStreamID uint32      // 记录当前链接最大的 streamID
}

// Free 释放持有的资源
func (d *decoder) Free() {
	bufpool.Release(d.rbuf)
	for _, stream := range d.streams {
		stream.Free()
	}
}

// Decode 从 zerocopy.Reader 解析 HTTP/2 二进制帧数据流 构建完整 RoundTrip
//
// # 协议特性要求
// - 处理多路复用的 Stream 数据交织 即同一连接中交错传输的多个 Request / Response
// - 支持头部压缩 HPACK 解码
// - 具备帧解析错误恢复能力 在协议错误时重置流
//
// # HTTP/2 支持链接的多路复用 交互示例 客户端发起的 Stream 为单数递增
//
// # Client                               # Server
// |                                      |
// |─── HEADERS[S1](:method=GET) ───────► | 启动 S1 请求
// |                                      |
// |────── HEADERS[S3](:method=POST) ───► | 同时启动 S3 请求
// |                                      |
// | ◄───── HEADERS[S1](status=200) ──────| S1 响应头到达
// |                                      |
// | ◄─── DATA[S1](chunk1) ───────────────| S1 数据分片
// |                                      |
// |─── DATA[S3](chunk3) ───────────────► | S3 请求体继续发送
// |                                      |
// | ◄─── HEADERS[S3](status=404) ────────| S3 响应头到达
// |                                      |
// | ◄───────── DATA[S3](error) ───────── | S3 错误数据
// |                                      |
// |─── RST_STREAM[S1] ─────────────────► | 主动终止 S1
// |                                      |
// | ◄───────── RST_STREAM[S3] ───────────| 服务端终止 S3
// |                                      |
//
// 从 HTTP2 Connection 的视角来看 每个 stream 初始化为两个不同的 streamDecoder
// 分别负责 HTTP2/request 和 HTTP2/response 的解析 如果解析成功后会向上层提交一个 *roundtrip.Object
// 上层拿到 *roundtrip.Object 会进行请求的配对 成功则代表成功捕获到一次 HTTP2 请求
//
// # Decoder 无法很好地测量流式通信 即建长链后一直处于数据的传输状态 由于客户端一直处于被动接收
// 数据的状态 所以无法确定 `请求发起的时间`
//
// 为了尽量模拟近似的 `请求时间`
// Request.Time 从发送的第一个数据包开始计时
// Response.Time 从接收的最后一个数据包停止计时
func (d *decoder) Decode(r zerocopy.Reader, t time.Time) ([]*role.Object, error) {
	d.t0 = t

	b, err := r.Read(common.ReadWriteBlockSize)
	if err != nil {
		return nil, nil
	}
	defer d.rbuf.Reset()

	var cut bool // 标识上一轮是否是待拼接数据
	var objs []*role.Object
	for len(b) > 0 {
		// 如果已经出现过两次拼接 返回解析错误
		if d.partial > 1 {
			return nil, errInvalidBytes
		}

		// 如果上一轮待拼接的数据 则追加在开头
		// 使用 buffer 避免重复分配内存
		if d.partial == 1 {
			d.rbuf.Write(d.tail)
			d.rbuf.Write(b)
			b = d.rbuf.Bytes()
		}

		data := &streamData{}

		// 正常从 header 开始解析
		if d.prevData.lackN == 0 {
			cut = false
			data, err = d.decodeHeader(b)
			if err != nil {
				// 仅有一次拼接机会
				if d.partial == 1 {
					return nil, nil
				}
				return nil, err
			}
		} else {
			// 如果上一轮解析中数据还没读取完毕
			cut = true
			data.id = d.prevData.id

			// 如果此时的 buf 的内容要大于上一轮未消费的部分
			// 仅消费所需的部分
			if uint32(len(b)) >= d.prevData.lackN {
				data.data = b[:d.prevData.lackN]
				data.lackN = 0
				data.tail = b[d.prevData.lackN:]
			} else {
				// 否则标记为已完全消费
				data.data = b
				data.lackN = d.prevData.lackN - uint32(len(b))
				data.tail = nil
			}
		}

		// 状态重置
		d.partial = 0
		d.prevData.id = data.id
		d.prevData.lackN = data.lackN

		b = data.tail

		sd := d.getOrCreateStream(data.id)
		obj, err := sd.Decode(cut, data.data, t)
		if err != nil {
			data = &streamData{} // 避免数据乱流 重置状态
			continue
		}

		if obj == nil {
			continue
		}
		objs = append(objs, obj)

		if sd.End() {
			d.deleteStream(sd.id)
		}
	}

	return objs, nil
}

type option struct {
	trailerKeys []string
}
type Option func(o *option)

func WithTrailersOpt(keys ...string) Option {
	return func(o *option) {
		o.trailerKeys = keys
	}
}

func NewDecoder(st socket.Tuple, serverPort socket.Port, opts ...Option) protocol.Decoder {
	opt := &option{}
	for _, f := range opts {
		f(opt)
	}
	return &decoder{
		st:         st.ToRaw(),
		serverPort: serverPort,
		opt:        opt,
		hfd:        NewHeaderFieldDecoder(opt.trailerKeys...),
		rbuf:       bufpool.Acquire(),
		prevData:   &streamData{},
		streams:    make(map[uint32]*streamDecoder),
	}
}

func (d *decoder) getOrCreateStream(id uint32) *streamDecoder {
	if sd, ok := d.streams[id]; ok {
		return sd
	}

	// stream 清理机制
	// 清理 id 最小的 stream 避免 streamid 未正常结束导致泄漏
	if len(d.streams) >= MaxConcurrentStreams {
		minV := uint32(math.MaxUint32)
		for sid := range d.streams {
			if sid <= 0 {
				continue
			}
			if minV > sid {
				minV = sid
			}
		}

		if sd, ok := d.streams[minV]; ok {
			sd.Free() // 释放资源再删除
			delete(d.streams, minV)
		}
	}

	sd := newStreamDecoder(id, d.st, d.serverPort, d.hfd)
	d.streams[id] = sd
	return sd
}

func (d *decoder) deleteStream(id uint32) {
	if sd, ok := d.streams[id]; ok {
		sd.Free() // 删除流之前需要释放资源
		delete(d.streams, id)
	}
}

type streamData struct {
	id    uint32
	data  []byte
	tail  []byte
	lackN uint32
}

// decodeHeader decoder 主要负责读取 HTTP2 中的 Header 并进行 streams 的分发
func (d *decoder) decodeHeader(b []byte) (*streamData, error) {
	// HTTP/2 在建链的时候会先发送 Connection Preface 数据包用于确认双方都支持 HTTP/2 协议
	// 此数据包明文传输
	if bytes.HasSuffix(b, connPreface) {
		return nil, errSkipConnPreface
	}

	// header 长度不足则 Clone 传入字节 留着下一轮拼接至头部解析
	if len(b) < headerLength {
		d.partial++
		d.tail = bytes.Clone(b) // 必须拷贝内存
		return nil, errDecodeHeader
	}

	// 前 3 个字节为 Header Length 即 24 位无符号整数
	// 这里使用 uint32 存储
	payloadLen := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
	if payloadLen > maxPayloadSize {
		d.partial++ // 连续两次则上层需要判为异常
		return nil, errDecodeHeader
	}

	var data []byte
	var tail []byte
	var lackN uint32

	total := headerLength + payloadLen // 计算总长度
	// 此时的 b 长度大于总长度
	// 则表示在本轮解析内可以读取到完整的数据包
	// tail 是剩余未读取的数据 留给下一轮解析
	if uint32(len(b)) >= total {
		data = b[:total]
		tail = b[total:]
	} else {
		// 不能读取完整的数据包 记录还差多少字节
		data = b
		lackN = total - uint32(len(b))
	}

	streamID := binary.BigEndian.Uint32(b[5:9]) & headerMask

	// 防御机制
	// 正常情况下请求的 StreamID 应该是单调递增的 Step 为 2
	// 不会突然新增一个大于之前非常多的 StreamID 此时大概率是流乱序了
	if d.maxStreamID < streamID {
		if streamID > d.maxStreamID+MaxConcurrentStreams*2 {
			return nil, errDecodeHeader
		}
		d.maxStreamID = streamID
	}
	return &streamData{
		id:    streamID,
		data:  data,
		tail:  tail,
		lackN: lackN,
	}, nil
}
