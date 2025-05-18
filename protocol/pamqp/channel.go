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
	"encoding/binary"
	"math"
	"time"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol/role"
)

const (
	PROTO = "AMQP"
)

var (
	errInvalidBytes      = newError("invalid bytes")
	errDecodeHeader      = newError("decode header failed")
	errDecodeString      = newError("decode string failed")
	errDecodeClassMethod = newError("decode class method failed")
)

type channelDecoder struct {
	id         uint16
	t0         time.Time
	st         socket.TupleRaw
	frameType  uint8
	serverPort socket.Port

	cm      classMethod
	packet  *Packet
	state   state
	readall bool
	errCode uint16

	payloadLen      uint32
	payloadConsumed uint32
	contentSize     uint64
	contentConsumed uint64

	waitContentHeader bool
	drainBytes        int
	reqTime           time.Time
	closed            bool
}

func newChannelDecoder(id uint16, st socket.TupleRaw, serverPort socket.Port) *channelDecoder {
	return &channelDecoder{
		id:         id,
		st:         st,
		serverPort: serverPort,
	}
}

// Decode 解析 AMQP 数据包 当且仅当 Request / Response 结束时归档 Object
func (cd *channelDecoder) Decode(b []byte, t time.Time) (*role.Object, error) {
	cd.t0 = t

	// 解析 header 获取 payload / flags 等信息
	if cd.state == stateDecodeHeader {
		if len(b) < headerHeadLength {
			return nil, errDecodeHeader
		}
		err := cd.decodeHeader(b[:headerHeadLength])
		if err != nil {
			return nil, err
		}
		b = b[headerHeadLength:]              // 切割剩余数据
		cd.payloadConsumed -= headerEndLength // header end 会在 payload 计算 consumed 被记进去 所以这里需要先减去
	}

	complete, err := cd.decodePayload(b)
	if err != nil {
		cd.reset() // 所有错误均会 reset 尽早修正数据流
		return nil, err
	}
	if complete {
		return cd.archive(), nil
	}
	return nil, nil
}

func (cd *channelDecoder) Free() {
	cd.packet = nil
}

func (cd *channelDecoder) Closed() bool {
	return cd.closed
}

// decodeHeader 解析 AMQP 的数据帧 Header
func (cd *channelDecoder) decodeHeader(b []byte) error {
	cd.drainBytes += len(b)
	payloadLen := binary.BigEndian.Uint32(b[3:7]) // 上层已经判断其长度了 可直接取值
	if payloadLen > maxPayloadSize {
		return errDecodeHeader
	}

	if cd.isClient() {
		cd.reqTime = cd.t0 // 将第一个数据包当做请求时间
	}

	cd.state = stateDecodePayload
	cd.frameType = b[0]
	cd.payloadLen = payloadLen
	return nil
}

func (cd *channelDecoder) decodePayload(b []byte) (bool, error) {
	cd.drainBytes += len(b)
	cd.payloadConsumed += uint32(len(b))
	cd.readall = cd.payloadLen == cd.payloadConsumed

	// 避免 contentBody size 未消费完而没有进入下一轮的 header 逻辑
	// 需要将其标记为 stateDecodeHeader
	if cd.readall {
		cd.state = stateDecodeHeader
		cd.payloadConsumed = 0
	}

	var err error
	switch cd.frameType {
	case frameMethod:
		err = cd.decodeFrameMethod(b)

	case frameContentHeader:
		err = cd.decodeFrameContentHeader(b)

	case frameContentBody:
		cd.decodeFrameContentBody()

	case frameHeartbeat:
	}

	if cd.waitContentHeader {
		cd.waitContentHeader = false // header 只会等待一次
		return false, nil
	}

	if cd.contentSize > 0 {
		return cd.readall && cd.contentSize == cd.contentConsumed, err
	}
	return cd.readall, err
}

func (cd *channelDecoder) isClient() bool {
	return cd.st.DstPort == uint16(cd.serverPort)
}

func (cd *channelDecoder) archive() *role.Object {
	ncm := &NamedClassMethod{
		Class:  classNames[cd.cm.ClassID],
		Method: classMethods[cd.cm],
	}

	if cd.isClient() {
		obj := role.NewRequestObject(&Request{
			ChannelID:   cd.id,
			FrameType:   frameNames[cd.frameType],
			Size:        cd.drainBytes,
			Proto:       PROTO,
			Time:        cd.reqTime,
			Host:        cd.st.SrcIP,
			Port:        cd.st.SrcPort,
			Packet:      cd.packet,
			ClassMethod: ncm,
			ErrCode:     matchErrCode(cd.errCode),
		})
		cd.reset()
		return obj
	}

	obj := role.NewResponseObject(&Response{
		ChannelID:   cd.id,
		FrameType:   frameNames[cd.frameType],
		Size:        cd.drainBytes,
		Proto:       PROTO,
		Time:        cd.t0,
		Host:        cd.st.SrcIP,
		Port:        cd.st.SrcPort,
		Packet:      cd.packet,
		ClassMethod: ncm,
		ErrCode:     matchErrCode(cd.errCode),
	})
	cd.reset()
	return obj
}

func (cd *channelDecoder) reset() {
	cd.frameType = 0
	cd.payloadLen = 0
	cd.payloadConsumed = 0
	cd.drainBytes = 0
	cd.readall = false
	cd.contentSize = 0
	cd.contentConsumed = 0
	cd.waitContentHeader = false
	cd.state = stateDecodeHeader
	cd.cm = classMethod{}
}

// decodeFrameMethod 解析 MethodFrame 内存布局如下:
//
// ┌───────────────┬─────────────────────┬─────────────────────────────┬───────┐
// │ Channel ID    │ Payload Size        │ Method Payload              │ Frame │
// │ (2 bytes)     │ (4 bytes)           │ (ClassID + MethodID + Args) │ End   │
// │               │                     │  2 bytes   2bytes           │ (0xCE)│
// ├───────────────┼─────────────────────┼─────────────────────────────┼───────┤
// │ 0x00 0x01     │ 0x00 0x00 0x00 0x0C │ 0x00 0x3C 0x00 0x28 ...     │ 0xCE  │
// └───────────────┴─────────────────────┴─────────────────────────────┴───────┘
//
// Method 的关键维度 如 `ExchangeName / RoutingKey / QueueName` 依赖 FieldRequests 的解析
//
// 对于其他 `非重要` 的字段 节省 CPU 不做判断
func (cd *channelDecoder) decodeFrameMethod(b []byte) error {
	if len(b) < 4 {
		return errInvalidBytes
	}

	cm := classMethod{
		ClassID:  binary.BigEndian.Uint16(b[0:2]),
		MethodID: binary.BigEndian.Uint16(b[2:4]),
	}

	// 需确保 classMethod 是合法的
	_, ok := classMethods[cm]
	if !ok {
		return errDecodeClassMethod
	}
	cd.cm = cm

	// 同时对于部分会跟随 ContentHeader 的 method 也需要做额外标记
	// 避免计算 bodySize 时出错
	_, ok = classMethodNeedContentHeader[cm]
	if ok {
		cd.waitContentHeader = true
	}

	// channel 关闭标记
	// 用于后续的流的清理操作
	if cd.cm.ClassID == classChannel && cd.cm.MethodID == 40 {
		cd.closed = true // 此状态不会重置
	}

	fr, ok := fieldRequestMap[cd.cm]
	if ok && len(b) > 4 {
		return cd.decodeFieldRequests(b[4:], fr)
	}
	return nil
}

// decodeFrameContentHeader 解析 ContentHeader 帧 内存布局
//
// ┌───────────────┬─────────────────────┬───────────────────────────────────────┬───────┐
// │ Channel ID    │ Payload Size        │ Header Payload                        │ Frame │
// │ (2 bytes)     │ (4 bytes)           │ (ClassID + BodySize + Props)          │ End   │
// │               │                     │  2 bytes   8 bytes                    │ (0xCE)│
// ├───────────────┼─────────────────────┼───────────────────────────────────────┼───────┤
// │ 0x00 0x01     │ 0x00 0x00 0x00 0x20 │ 0x00 0x3C 0x00 0x00 0x00 0x00 0x00... │ 0xCE  │
// └───────────────┴─────────────────────┴───────────────────────────────────────┴───────┘
//
// 数据包如果比较大则可能会出现 1 FrameContentHeader + N FrameContentBody 的情况
// 此时只有当所有 FrameContentBody 解析完成后才会标记结束 需要将 bodySize 记录下来
// Props 字段不做解析
func (cd *channelDecoder) decodeFrameContentHeader(b []byte) error {
	if len(b) < 12 {
		return errInvalidBytes
	}

	classID := binary.BigEndian.Uint16(b[0:2])
	bodySize := binary.BigEndian.Uint64(b[4:12])
	if bodySize > maxPayloadSize {
		return errDecodeHeader
	}

	_, ok := classNames[classID]
	if !ok {
		return errDecodeClassMethod
	}

	cd.contentSize = bodySize
	if cd.cm.ClassID == 0 {
		cd.cm.ClassID = classID // Header 里只有 ClassID 无 MethodID
	}
	return nil
}

// decodeFrameContentBody 解析 ContentBody 帧 内存布局
//
// ┌───────────────┬─────────────────────┬────────────────────────────┬───────┐
// │ Channel ID    │ Payload Size        │ Raw Message Data           │ Frame │
// │ (2 bytes)     │ (4 bytes)           │ (N bytes)                  │ End   │
// │               │                     │                            │ (0xCE)│
// ├───────────────┼─────────────────────┼────────────────────────────┼───────┤
// │ 0x00 0x01     │ 0x00 0x00 0x00 0x0A │ 0x48 0x65 0x6C 0x6C 0x6F...│ 0xCE  │
// └───────────────┴─────────────────────┴────────────────────────────┴───────┘
//
// ContentBody 帧具有以下特性:
// 1. 数据连续性: 必须跟随在相同通道的 ContentHeader 帧之后
// 2. 分片规则: 若消息体超过单帧容量 需拆分为多个 Body 帧
//   - 所有 Body 帧数据总长度必须等于 Header 中的 BodySize
//   - 分片示例: ContentHeader.BodySize=300KB → 3 个 Body 帧 (128KB + 128KB + 44KB)
//
// 仅做 size 记录 不对 body 具体内容进行处理
func (cd *channelDecoder) decodeFrameContentBody() {
	cd.contentConsumed += uint64(cd.payloadLen)
}

// Packet 代表着 AMQP 通信协议中的关键字段
type Packet struct {
	ExchangeName string // 交换机名称
	RoutingKey   string // 路由键
	QueueName    string // 队列名称
}

// decodeFieldRequests 解析数据帧中的 `重要` 字段
//
// 规则参见 classmethod.go 此方式可以让字段解析变得更加灵活 允许配置化的定义解析方案
func (cd *channelDecoder) decodeFieldRequests(b []byte, fr fieldRequest) error {
	var skip int
	var offset int

	var nothing string
	var exchangeName string
	var routingKey string
	var queueName string

	decodeString := func(p *string) error {
		var err error
		if len(b) <= skip {
			return errInvalidBytes
		}
		*p, offset, err = decodeShortString(b[skip:])
		if err != nil {
			return err
		}
		skip += offset
		return nil
	}

	ops := fr.ops
	round := 0
	for i := 0; i < len(ops); i++ {
		round++
		switch ops[i] {
		case opSkipUint8:
			skip += 1
		case opSkipUint16:
			skip += 2
		case opSkipUint64:
			skip += 8
		case opSkipShortString:
			if err := decodeString(&nothing); err != nil {
				return err
			}

		case opExchangeName:
			if err := decodeString(&exchangeName); err != nil {
				return err
			}
		case opRoutingKey:
			if err := decodeString(&routingKey); err != nil {
				return err
			}
		case opQueueName:
			if err := decodeString(&queueName); err != nil {
				return err
			}

		case opErrCode:
			if offset+2 >= len(b) {
				return errInvalidBytes
			}
			cd.errCode = binary.BigEndian.Uint16(b[offset : offset+2])

		default:
			round--
		}
	}

	// 解析轮次要求等于 ops 长度
	if round != len(ops) || skip > len(b) {
		return errInvalidBytes
	}

	cd.packet = &Packet{
		ExchangeName: exchangeName,
		RoutingKey:   routingKey,
		QueueName:    queueName,
	}
	return nil
}

func decodeShortString(b []byte) (string, int, error) {
	if len(b) < 1 {
		return "", 0, errDecodeString
	}

	n := b[0]
	if len(b) < 1+int(n) || 1+int(n) > math.MaxUint8 {
		return "", 0, errDecodeString
	}

	s := string(b[1 : 1+n])
	return s, 1 + int(n), nil
}
