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

package pamqp

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

const (
	frameMethod        = 0x01
	frameContentHeader = 0x02
	frameContentBody   = 0x03
	frameHeartbeat     = 0x08
)

var frameNames = map[byte]string{
	frameMethod:        "Method",
	frameContentHeader: "ContentHeader",
	frameContentBody:   "ContentBody",
	frameHeartbeat:     "Heartbeat",
}

// validateFrameType 校验帧类型是否合法
func validateFrameType(b byte) bool {
	switch b {
	case frameMethod, frameContentHeader, frameContentBody, frameHeartbeat:
		return true
	default:
		return false
	}
}

func newError(format string, args ...any) error {
	format = "amqp/decoder: " + format
	return errors.Errorf(format, args...)
}

// state 记录着 decoder 的处理状态
type state int

const (
	// stateDecodeHeader 处于解析 header 状态
	stateDecodeHeader state = iota

	// stateDecodePayload 处于解析 Payload 状态
	stateDecodePayload
)

const (
	// headerHeadLength header 头部长度
	// - FrameType (1B) + FrameSize (4B) + ChannelID(2B)
	headerHeadLength = 7

	// headerEndLength header 尾部长度
	// 0xCE
	headerEndLength = 1

	// maxPayloadSize 最大 payload 大小
	maxPayloadSize = 2147483647
)

type decoder struct {
	t0         time.Time
	st         socket.TupleRaw
	serverPort socket.Port

	rbuf     *bytes.Buffer
	prevData *channelData
	channels map[uint16]*channelDecoder

	tail    []byte // 尾部数据拼接 仅允许拼接一次 避免上一轮切割了部分数据
	partial uint8  // 标记上一轮的 header 是否待拼接
}

func NewDecoder(st socket.Tuple, serverPort socket.Port, _ common.Options) protocol.Decoder {
	return &decoder{
		st:         st.ToRaw(),
		serverPort: serverPort,
		rbuf:       bufpool.Acquire(),
		prevData:   &channelData{},
		channels:   make(map[uint16]*channelDecoder),
	}
}

type channelData struct {
	id    uint16
	data  []byte
	tail  []byte
	lackN uint32
}

// Decode 从 zerocopy.Reader 解析 AMQP 二进制帧数据流 构建完整 RoundTrip
//
// # 协议特性要求
// - 处理多路复用的 Channel 数据交织 同一连接中多个逻辑通道独立处理消息流
// - 支持帧类型自动识别（方法帧/内容头帧/内容体帧/心跳帧）
// - 具备通道级错误隔离能力 单个通道异常不影响其他通道
//
// # AMQP 通道多路复用示例 客户端与服务端通过不同通道并行操作
//
// # Client                               # Server
// |                                      |
// |─── Method[Ch1](Queue.Declare) ─────► | Channel1 声明队列
// |                                      |
// |────── Method[Ch2](Exchange.Bind) ──► | Channel2 绑定交换器
// |                                      |
// |◄── Method[Ch1](Declare-Ok) ───────── | Channel1 确认声明
// |                                      |
// |────── Header[Ch3]+Body (MsgA) ─────► | Channel3 发布消息
// |                                      |
// |◄── Header[Ch4]+Body (MsgB) ───────── | Channel4 推送消费消息
// |                                      |
// |────── Method[Ch3](Basic.Ack) ──────► | Channel3 确认消息处理
// |                                      |
// |◄── Method[Ch2](Bind-Ok) ──────────── | Channel2 确认绑定
// |                                      |
//
// # 时间测量说明
// - Publish.Time: 从首个 Method(Basic.Publish) 帧开始计时
// - Deliver.Time: 到最后一个 Body 帧到达完成计时
func (d *decoder) Decode(r zerocopy.Reader, t time.Time) ([]*role.Object, error) {
	d.t0 = t

	b, err := r.Read(common.ReadWriteBlockSize)
	if err != nil {
		return nil, nil
	}
	defer d.rbuf.Reset()

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

		data := &channelData{}

		// 正常从 header 开始解析
		if d.prevData.lackN == 0 {
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

		cd := d.getOrCreateChannel(data.id)
		obj, err := cd.Decode(data.data, t)
		if err != nil {
			data = &channelData{} // 避免数据乱流 重置状态
			continue
		}

		if obj == nil {
			continue
		}
		objs = append(objs, obj)

		if cd.Closed() {
			d.deleteChannel(cd.id)
		}
	}

	return objs, nil
}

func (d *decoder) Free() {
	bufpool.Release(d.rbuf)
}

func (d *decoder) getOrCreateChannel(id uint16) *channelDecoder {
	if sd, ok := d.channels[id]; ok {
		return sd
	}

	// channel 清理机制
	// 清理 id 最小的 channel 避免 channelid 未正常结束导致泄漏
	if len(d.channels) >= maxRecordSize {
		minV := uint16(math.MaxUint16)
		for cid := range d.channels {
			if cid <= 0 {
				continue
			}
			if minV > cid {
				minV = cid
			}
		}

		if cd, ok := d.channels[minV]; ok {
			cd.Free() // 释放资源再删除
			delete(d.channels, minV)
		}
	}

	sd := newChannelDecoder(id, d.st, d.serverPort)
	d.channels[id] = sd
	return sd
}

func (d *decoder) deleteChannel(id uint16) {
	if cd, ok := d.channels[id]; ok {
		cd.Free() // 删除 channel 之前需要释放资源
		delete(d.channels, id)
	}
}

// decodeHeader 解析 AMQP Frame Header 帧头结构
//
// AMQP 协议所有帧均以 7 字节的帧头开始 结构如下:
//
// ┌──────────────────────── AMQP Frame Header ──────────────────────┐
// │ 0                   1                   2                   3   │
// │ 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 │
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬─────────┤
// │ Frame │       Channel ID      │         Payload Size            │
// │ Type  │     (Big-Endian)      │        (Big-Endian)             │
// ├───────┴───────┴───────┴───────┴───────┴───────┴───────┴─────────┤
// │                                                                 │
// │ 0x01 0x00 0x01 0x00 0x00 0x00 0x80                              │
// └─────────────────────────────────────────────────────────────────┘
//
// * Frame Type (1 Byte):
//   - 0x01: METHOD FRAME (方法帧，携带 AMQP 命令)
//   - 0x02: HEADER FRAME (内容头帧，描述消息属性)
//   - 0x03: BODY FRAME (内容体帧，携带消息二进制数据)
//   - 0x08: HEARTBEAT FRAME (心跳帧 无 Payload)
//
// * Channel ID (2 Bytes, Big-Endian):
//   - 0x0000: 系统保留通道（用于全局连接操作）
//   - 0x0001~0xFFFF: 业务通道（每个通道独立处理消息流）
//
// * Payload Size (4 Bytes, Big-Endian):
//   - 载荷部分总字节数（不包含帧头和结束符）
//   - 最大允许值由 Connection.Tune 协商（默认 131072=128KB）
//
// 帧头后紧跟 Payload 和 Frame End (0xCE)，完整帧结构：
// ┌───────┬───────────────┬───────────────────┬───────┐
// │ Header (7B)           │ Payload (N Bytes) │ 0xCE  │
// └───────┴───────────────┴───────────────────┴───────┘
func (d *decoder) decodeHeader(b []byte) (*channelData, error) {
	if len(b) < headerHeadLength {
		d.partial++
		d.tail = bytes.Clone(b)
		return nil, errDecodeHeader
	}

	var data []byte
	var tail []byte
	var lackN uint32

	if !validateFrameType(b[0]) {
		return nil, errDecodeHeader
	}

	channelID := binary.BigEndian.Uint16(b[1:3])
	payloadLen := binary.BigEndian.Uint32(b[3:7])

	if payloadLen > maxPayloadSize {
		d.partial++ // 连续两次则上层需要判为异常
		return nil, errDecodeHeader
	}

	total := headerHeadLength + payloadLen + headerEndLength // 计算总长度
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

	return &channelData{
		id:    channelID,
		data:  data,
		tail:  tail,
		lackN: lackN,
	}, nil
}
