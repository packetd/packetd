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

package pkafka

import (
	"bytes"
	"encoding/binary"
	"math"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

const (
	PROTO = "Kafka"
)

func newError(format string, args ...any) error {
	format = "kafka/decoder: " + format
	return errors.Errorf(format, args...)
}

var (
	errInvalidBytes        = newError("invalid bytes")
	errDecodeHeader        = newError("decode header failed")
	errDecodeString        = newError("decode string failed")
	errDecodeCompactString = newError("decode compactString failed")
)

// state 记录着 decoder 的处理状态
type state uint8

const (
	// stateDecodeHeader 初始值 处于 header 解析状态
	stateDecodeHeader state = iota

	// stateDecodePayload 处于 payload 解析状态
	stateDecodePayload
)

const (
	// reqMinHeaderLength header 最小长度
	reqMinHeaderLength = 14

	// rspMinHeaderLength header 最小长度
	rspMinHeaderLength = 8
)

type decoder struct {
	st         socket.TupleRaw
	t0         time.Time
	state      state
	serverPort socket.Port
	reqTime    time.Time

	payloadLen      uint32
	payloadConsumed uint32

	reqHdr *requestHeader
	rspHdr *responseHeader

	readall    bool
	drainBytes int
	ak         apiKey
	errCode    errorCode
	topicDone  bool
	packet     *Packet

	tail    []byte // 尾部数据拼接 仅允许拼接一次 避免上一轮切割了部分数据
	partial uint8
}

func NewDecoder(st socket.Tuple, serverPort socket.Port) protocol.Decoder {
	return &decoder{
		st:         st.ToRaw(),
		serverPort: serverPort,
		ak:         math.MaxUint16,
		errCode:    math.MaxInt16,
	}
}

func (d *decoder) Free() {
	d.tail = nil
}

// Decode 持续从 zerocopy.Reader 解析 Kafka 协议数据流，构建并返回 RoundTrip 对象
//
// # 解码器需具备容错和自恢复能力：
// 1. 遇到不完整数据时记录错误并等待后续数据
// 2. 校验失败时重置解析状态并寻找下一个有效帧
// 3. 支持协议版本协商（0.8.0 ~ 3.x+）
//
// # 在 Kafka 中作为一个请求-响应协议以如下方式使用
//
// - 客户端发送 Kafka RequestAPI 到服务端
// - 服务器端根据 API 以及 Version 处理并响应不同的数据包 以 Produce 举例
//
// # RoundTrip 构建规则
// +---------------------+                      +------------------+
// |      Producer       |                      |       Broker     |
// +---------------------+                      +------------------+
// | PRODUCE Request     |  ---------------->   |                  |
// | topic: orders       |                      |                  |
// | partition: 3        |                      |                  |
// | messages: [...]     |                      |                  |
// +---------------------+                      +------------------+
// |                     |  <----------------   | PRODUCE Response |
// | base_offset: 142857 |                      | throttle_ms: 0   |
// +---------------------+                      +------------------+
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

	var complete bool

	// 持续解析读取到的所有字节 直到 EOF
	for len(b) > 0 {
		// 如果已经出现过两次拼接 返回解析错误
		if d.partial > 1 {
			return nil, errInvalidBytes
		}

		if d.partial == 1 {
			b = append(d.tail, b...)
		}
		b, complete, err = d.decode(b)
		if err != nil {
			if d.partial == 1 {
				continue
			}
			d.reset() // 错误即重置
			return nil, err
		}

		d.partial = 0 // 当轮次解析没问题
		if !complete {
			continue
		}

		return d.archive(), nil
	}
	return nil, nil
}

// reset 重置单次请求状态
func (d *decoder) reset() {
	d.topicDone = false
	d.payloadConsumed = 0
	d.payloadLen = 0
	d.drainBytes = 0
	d.state = stateDecodeHeader
	d.readall = false
	d.ak = math.MaxUint16
	d.errCode = math.MaxInt16
	d.packet = nil
}

// archive 归档请求
func (d *decoder) archive() []*role.Object {
	if d.isClient() {
		obj := role.NewRequestObject(&Request{
			CorrelationID: d.reqHdr.correlationID,
			Size:          d.drainBytes,
			Proto:         PROTO,
			Time:          d.reqTime,
			Host:          d.st.SrcIP,
			Port:          d.st.SrcPort,
			Packet:        d.packet,
		})
		d.reset()
		return []*role.Object{obj}
	}

	obj := role.NewResponseObject(&Response{
		CorrelationID: d.rspHdr.correlationID,
		Size:          d.drainBytes,
		Time:          d.t0,
		Proto:         PROTO,
		Host:          d.st.SrcIP,
		Port:          d.st.SrcPort,
		ErrorCode:     errCodes[d.errCode],
	})
	d.reset()
	return []*role.Object{obj}
}

// decode 真正的解析入口
func (d *decoder) decode(b []byte) ([]byte, bool, error) {
	// 首先处理 Header 部分 需要区分 client/server
	if d.state == stateDecodeHeader {
		if d.isClient() {
			if len(b) < reqMinHeaderLength {
				d.partial++
				d.tail = bytes.Clone(b) // 留着下轮拼接
				return nil, false, errDecodeHeader
			}

			// 解析 client Request
			reqHdr, err := d.decodeRequestHeader(b)
			if err != nil {
				return nil, false, err
			}

			// 追加 clientID 字符串长度
			d.drainBytes += reqMinHeaderLength + len(reqHdr.clientID)
			d.payloadConsumed += 10 + uint32(len(reqHdr.clientID))
			d.payloadLen = uint32(reqHdr.length)

			d.reqHdr = reqHdr
			d.state = stateDecodePayload
			b = b[reqMinHeaderLength+len(reqHdr.clientID):]
		} else {
			if len(b) < rspMinHeaderLength {
				d.partial++
				d.tail = bytes.Clone(b)
				return nil, false, errDecodeHeader
			}

			// 解析 server Request
			rspHdr, err := d.decodeResponseHeader(b)
			if err != nil {
				return nil, false, err
			}

			d.drainBytes += rspMinHeaderLength
			d.payloadConsumed += 4
			d.payloadLen = uint32(rspHdr.length)

			d.rspHdr = rspHdr
			d.state = stateDecodePayload
			b = b[rspMinHeaderLength:]
		}
	}

	b, complete, err := d.decodePayload(b)
	if err != nil {
		return nil, false, err
	}
	return b, complete, nil
}

func (d *decoder) isClient() bool {
	return uint16(d.serverPort) == d.st.DstPort
}

// decodePayload 解析协议 Payload 不定长度
//
// Payload 可能包含【多种】类型的数据包 需要按 Length-Payload 的顺序交替解析
func (d *decoder) decodePayload(b []byte) ([]byte, bool, error) {
	if len(b) == 0 {
		return nil, false, nil
	}

	// 仅记录服务端 Error Code
	// errCode 初始化值为 math.MaxInt16
	if !d.isClient() && int16(d.errCode) == math.MaxInt16 {
		if len(b) >= 2 {
			d.errCode = errorCode(binary.BigEndian.Uint16(b[:2]))
		}
	}

	n := d.payloadConsumed + uint32(len(b))
	// 刚好能够拼接成一个数据包
	if n == d.payloadLen {
		d.payloadConsumed += uint32(len(b))
		d.drainBytes += len(b)
		d.state = stateDecodeHeader
		d.readall = true
		complete, err := d.decodePacket(b)
		return nil, complete, err
	}

	// 剩余的数据已经超过一个完整的 payload 所需要的字节
	// 则需要消费剩下的内容并将 tail 返回
	if n > d.payloadLen {
		if d.payloadLen < d.payloadConsumed {
			return nil, false, errInvalidBytes
		}
		consumed := d.payloadLen - d.payloadConsumed
		d.payloadConsumed += consumed
		d.drainBytes += int(consumed)
		d.state = stateDecodeHeader
		d.readall = true
		complete, err := d.decodePacket(b[:consumed])
		return b[consumed:], complete, err
	}

	d.payloadConsumed += uint32(len(b))
	d.drainBytes += len(b)
	complete, err := d.decodePacket(b)
	return nil, complete, err
}

// decodePacket 根据 API/Version 进行真正的协议解析
func (d *decoder) decodePacket(b []byte) (bool, error) {
	if !d.isClient() {
		_, ok := apiKeys[d.ak]
		if ok && d.packet == nil {
			d.updatePacket("", "") // 服务端请求仅需记录长度
		}
		return d.readall, nil
	}

	// 不允许非法 apikey
	if _, ok := apiKeys[d.ak]; !ok {
		return false, newError("api (%d) not found", d.ak)
	}

	var err error
	var decoded bool // 记录是否已经处理过
	if _, ok := topicRequestMap[d.ak]; ok {
		err = d.decodeTopicRequests(b)
		decoded = true
	}
	if _, ok := fieldRequestMap[d.ak]; ok {
		err = d.decodeFieldRequest(b)
		decoded = true
	}

	// 当 b 还没被处理过时 可能是无 group/topic 的请求
	// 那直接生成 packet 即可
	if !decoded && d.packet == nil {
		_, ok := apiKeys[d.ak]
		if ok && d.packet == nil {
			d.updatePacket("", "")
		}
	}

	if err != nil {
		return false, err
	}

	// 当且仅当已经读取完请求所有数据并构建成 packet 才算解析完成
	if d.readall && d.packet != nil {
		return true, nil
	}
	return false, nil
}

type requestHeader struct {
	apiKey        apiKey
	apiVersion    int16
	correlationID int32
	clientID      string
	length        int32
}

// decodeRequestHeader 解析 Request 请求头 数据布局如下
//
// # Request 请求分为 Header 和 Payload 两部分 这里以 MetaDataAPI 举例
//
// ┌──────────────────────── Kafka Request ────────────────────────┐
// │ 0                   1                   2                   3 │
// │ 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │                     Request Length (4 Bytes)                  │
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │          API Key (2 Bytes)    │   API Version (2 Bytes)       │
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │                     Correlation ID (4 Bytes)                  │
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │  Client ID Length (2 Bytes)   │  Client ID (Variable)         │
// ├─ ... ─┴───────┬───────┬───────┼───────┬───────┬───────┬───────┤
// │                     Request Body (Variable)                   │
// │                                                               │
// │  Example Metadata Request Body:                               │
// │  ├───────┬───────┬───────┬───────┬───────┬───────┬───────┤    │
// │  │ Topics Count (4 Bytes)       │  [Topic 1 Structure]   │    │
// │  ├───────┴───────┼───────┬───────┼───────┬───────┬───────┤    │
// │  │ Topic Name Length (2) │ Topic Name (Var)      │ Partitions │
// └───────┴───────┴───────┴───────┴───────┴───────┴───────┴───────┘
//
// * Request Length (Int32): 整个请求帧的字节数（包含本字段之后的全部数据）
//   - 0x000000C8 表示后续 200 字节数据
//
// * API Key (Int16): 标识请求类型 详见 api.go
//   - 0x0000 : PRODUCE (生产消息)
//   - 0x0001 : FETCH (拉取消息)
//   - 0x0003 : METADATA (获取元数据)
//
// * API Version (Int16): 标识客户端使用的协议版本
//   - 0x0009 表示使用第 9 版协议格式
//
// * Correlation ID (Int32): 请求-响应匹配标识符（客户端生成唯一 ID）
//   - 响应帧会携带相同 ID（管理标识 ID 固定位 0）
//
// * Client ID Length (Int16): 客户端 ID 长度
//   - 0x0005 表示客户端 ID 的长度为 5
//
// * Client ID (UTF-8 String): 客户端 ID
//   - "kafka-producer-1"
//
// Payload 需根据 API Key / API Version 共同决定如何解析 详见 api.go
func (d *decoder) decodeRequestHeader(b []byte) (*requestHeader, error) {
	if len(b) < reqMinHeaderLength {
		return nil, errDecodeHeader
	}

	length := int32(binary.BigEndian.Uint32(b[:4]))
	ak := apiKey(binary.BigEndian.Uint16(b[4:6]))
	if _, ok := apiKeys[ak]; !ok {
		return nil, errDecodeHeader
	}
	d.ak = ak // apikey 在单次请求中需要持续记录

	apiVersion := int16(binary.BigEndian.Uint16(b[6:8]))
	correlation := int32(binary.BigEndian.Uint32(b[8:12]))

	clientIDLen := binary.BigEndian.Uint16(b[12:14])
	if int(clientIDLen+14) > len(b) {
		return nil, errDecodeHeader
	}
	// 避免数组溢出
	if 14+int(clientIDLen) > math.MaxUint16 {
		return nil, errDecodeHeader
	}

	clientID := string(b[14 : 14+clientIDLen])
	d.reqTime = d.t0
	return &requestHeader{
		apiKey:        ak,
		apiVersion:    apiVersion,
		correlationID: correlation,
		clientID:      clientID,
		length:        length,
	}, nil
}

type responseHeader struct {
	correlationID int32
	length        int32
}

// decodeResponseHeader 解析 Response 请求头 数据布局如下
//
// # Response 请求分为 Header 和 Payload 两部分 这里以 MetaDataAPI 举例
//
// ┌──────────────────────── Kafka Response ───────────────────────┐
// │ 0                   1                   2                   3 │
// │ 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │                    Response Length (4 Bytes)                  │
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │                    Correlation ID (4 Bytes)                   │
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │                    Response Body (Variable)                   │
// │                                                               │
// │  Example Metadata Response Body:                              │
// │  ├───────┬───────┬───────┬───────┬───────┬───────┬───────┤    │
// │  │ Throttle Time (4 Bytes)       │ Brokers Count (4)     │    │
// │  ├───────┴───────┬───────┬───────┼───────┬───────┬───────┤    │
// │  │ Broker ID (4) │ Host Len (2)  │ Host (Var)    │ Ports (4)   │
// │  ├───────┬───────┴───────┴───────┼───────┴───────┴───────┤    │
// │  │ ...                           │ Topics Count (4)      │    │
// └───────┴───────┴───────┴───────┴───────┴───────┴───────┴───────┘
//
// * Response Length (Int32): 整个响应帧的字节数（包含本字段之后的全部数据）
//   - 0x0000012C 表示后续 300 字节数据
//   - 特殊值 0xFFFFFFFF 表示长度超过 4GB（实际场景罕见）
//
// * Correlation ID (Int32): 与请求帧严格对应的匹配标识符
//   - 必须与请求中的 Correlation ID 完全相同 如请求 0xFEEDFACE → 响应必须返回 0xFEEDFACE
//
// Payload 需根据 API Key / API Version 共同决定如何解析
func (d *decoder) decodeResponseHeader(b []byte) (*responseHeader, error) {
	if len(b) < rspMinHeaderLength {
		return nil, errDecodeHeader
	}

	length := int32(binary.BigEndian.Uint32(b[:4]))
	correlation := int32(binary.BigEndian.Uint32(b[4:8]))
	return &responseHeader{
		correlationID: correlation,
		length:        length,
	}, nil
}

type Packet struct {
	API           string
	APIVersion    int16
	CorrelationID int32
	ClientID      string
	GroupID       string
	Topic         string
}

func (d *decoder) updatePacket(groupID, topic string) {
	d.packet = &Packet{
		API:           apiKeys[d.ak],
		APIVersion:    d.reqHdr.apiVersion,
		CorrelationID: d.reqHdr.correlationID,
		ClientID:      d.reqHdr.clientID,
		GroupID:       groupID,
		Topic:         topic,
	}
}

// decodeFieldRequest 提供了一种按配置解析不同 API 不同版本字段的能力
//
// 目前支持多种 op 类型 如 Int16/Int32/Int64/...
func (d *decoder) decodeFieldRequest(b []byte) error {
	if d.topicDone {
		return nil
	}

	// 提取解析规则
	opField, ok := matchFieldRequest(d.ak, d.reqHdr.apiVersion)
	if !ok {
		return newError("field request/version=(%d/%d) not found", d.ak, d.reqHdr.apiVersion)
	}

	var skip int
	var round int
	var groupID string
	for i := 0; i < len(opField.ops); i++ {
		switch opField.ops[i] {
		case opInt16:
			if len(b) < skip+2 {
				return errInvalidBytes
			}
			skip += 2
			round++

		case opInt32:
			if len(b) < skip+4 {
				return errInvalidBytes
			}
			skip += 4
			round++

		case opInt64:
			if len(b) < skip+8 {
				return errInvalidBytes
			}
			skip += 8
			round++

		case opUvarint:
			if len(b) < skip+1 {
				return errInvalidBytes
			}
			_, n := binary.Uvarint(b)
			skip += n
			round++

		case opString, opGroupID:
			var s string
			var offset int
			var err error

			// string 可能会进行压缩 这里分两个路径进行解析
			if opField.compact {
				s, offset, err = decodeCompactStringType(b[skip:])
				if err != nil {
					return errInvalidBytes
				}

			} else {
				s, offset, err = decodeStringType(b[skip:], true)
				if err != nil {
					return errInvalidBytes
				}
			}

			skip += offset
			round++
			if opField.ops[i] == opGroupID {
				groupID = toUtf8String([]byte(s)) // 如果是 group 则需要记录下来
			}

		default:
		}
	}

	// 解析轮次要求等于 ops 长度
	if round != len(opField.ops) || skip > len(b) {
		return errInvalidBytes
	}

	if !opField.withTopic {
		d.updatePacket(groupID, "")
		d.topicDone = true
		return nil
	}

	var topic string
	var err error
	if opField.compact {
		_, l := binary.Uvarint(b[skip:])
		if l > 0 {
			skip += l
			topic, _, err = decodeCompactStringType(b[skip:])
			if err != nil {
				return err
			}
		}
	} else {
		n := int32(binary.BigEndian.Uint32(b[skip : skip+4]))
		if n == -1 {
			d.updatePacket(groupID, "")
			d.topicDone = true
			return nil
		}

		topic, _, err = decodeStringType(b[skip+4:], true)
		if err != nil {
			return err
		}
	}

	d.updatePacket(groupID, topic)
	d.topicDone = true
	return nil
}

// decodeTopicRequests 仅解析 Request 的 Topic 字段
func (d *decoder) decodeTopicRequests(b []byte) error {
	if d.topicDone {
		return nil
	}

	tr, ok := matchTopicRequest(d.ak, d.reqHdr.apiVersion)
	if !ok {
		return newError("topic request/version=(%d/%d) not found", d.ak, d.reqHdr.apiVersion)
	}

	skip := tr.skip
	if len(b) < skip+4 {
		return newError("decode %d request failed", d.ak)
	}

	if tr.topicType == topicTypeUUID {
		uuid := binary.BigEndian.Uint16(b[skip : skip+2])
		d.updatePacket("", strconv.Itoa(int(uuid)))
		d.topicDone = true
		return nil
	}

	n := int(binary.BigEndian.Uint32(b[skip : skip+4]))
	if n == -1 {
		d.updatePacket("", "") // 空数组则 topic 置空
		d.topicDone = true
		return nil
	}

	// 如果数组长度非空则一定会有 topic 此时不允许 null
	topic, _, err := decodeStringType(b[skip+4:], false)
	if err != nil {
		return err
	}

	d.topicDone = true
	d.updatePacket("", topic)
	return nil
}

// decodeStringType 解码 Kafka 标准字符串格式
//
// nullable 指定是否允许空长度的字符串 返回解析后的字符串以及消耗的字节数
func decodeStringType(b []byte, nullable bool) (string, int, error) {
	if len(b) < 2 {
		return "", 0, errDecodeString
	}

	n := int(binary.BigEndian.Uint16(b[:2]))
	if n == -1 {
		if !nullable {
			return "", 0, errDecodeString
		}
		return "", 2, nil
	}

	if n+2 > len(b) || n < 0 {
		return "", 0, errDecodeString
	}
	return toUtf8String(b[2 : n+2]), n + 2, nil
}

// decodeCompactStringType 解码 Kafka 紧凑字符串格式
//
// 返回解析后的字符串以及消耗的字节数
func decodeCompactStringType(b []byte) (string, int, error) {
	length, n := binary.Uvarint(b)
	if n <= 0 {
		return "", 0, errDecodeCompactString
	}

	if length == 0 {
		return "", n, nil
	}

	l := int(length - 1)
	if n+l > len(b) {
		return "", 0, errDecodeCompactString
	}
	return toUtf8String(b[n : n+l]), n + l, nil
}

func toUtf8String(b []byte) string {
	dst := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if isCharNormalized(b[i]) {
			dst = append(dst, b[i])
		}
	}
	return string(dst)
}

func isCharNormalized(b byte) bool {
	switch b {
	case '_', '-', ':', '/':
		return true
	}

	if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') {
		return true
	}
	return false
}
