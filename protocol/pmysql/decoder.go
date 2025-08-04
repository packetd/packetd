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

package pmysql

import (
	"bytes"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/bufbytes"
	"github.com/packetd/packetd/internal/splitio"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

const (
	PROTO = "MySQL"
)

func newError(format string, args ...any) error {
	format = "mysql/decoder: " + format
	return errors.Errorf(format, args...)
}

var (
	errInvalidBytes    = newError("invalid bytes")
	errDecodeHeader    = newError("decode Header failed")
	errDecodeResponse  = newError("decode Response failed")
	errDecodeOKPacket  = newError("decode OKPacket failed")
	errDecodeErrPacket = newError("decode ErrPacket failed")
	errDecodeEOFPacket = newError("decode EOFPacket failed")
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
	// headerLength header 固定字节长度
	headerLength = 4

	// maxPayloadSize 单 payload 最大长度 超过此长度服务端会进行切片
	maxPayloadSize = 0xFFFFFF

	// maxStatementSize SQL 语句缓冲区大小
	maxStatementSize = 1024

	// maxErrMsgSize 避免超长 error message
	maxErrMsgSize = 256
)

type decoder struct {
	t0         time.Time
	st         socket.TupleRaw
	serverPort socket.Port

	reqTime time.Time
	state   state
	role    role.Role

	seqID           uint8
	payloadLen      uint32
	payloadConsumed uint32
	drainBytes      int
	eofPackets      int
	headers         int

	obj        any
	cmdType    uint8
	packetType uint8
	statement  *bufbytes.Bytes

	tail       []byte // 尾部数据拼接 仅允许拼接一次 避免上一轮切割了部分数据
	partial    uint8
	waitForRsp bool
}

func NewDecoder(st socket.Tuple, serverPort socket.Port, _ common.Options) protocol.Decoder {
	return &decoder{
		st:         st.ToRaw(),
		serverPort: serverPort,
		statement:  bufbytes.New(maxStatementSize), // 执行语句 buffer
	}
}

// reset 重置单次请求状态
func (d *decoder) reset() {
	d.role = ""
	d.payloadConsumed = 0
	d.payloadLen = 0
	d.state = stateDecodeHeader
	d.eofPackets = 0
	d.drainBytes = 0
	d.cmdType = 0
	d.seqID = 0
	d.headers = 0
	d.tail = nil
	d.partial = 0
	d.waitForRsp = false
	d.statement.Reset()
}

// Decode 从 zerocopy.Reader 中不断解析来自 Request / Response 的数据 并判断是否能构建成 RoundTrip
//
// # Decode 要求具备容错和自恢复能力 即当出现错误的时候能够适当重置
//
// 解析 MySQL 标准协议 Request 只解析 Header 以及 CommandType 而 Response 会解析 EOFPacket / OKPacket / ErrorPacket
//
// - OKPacket: 正常的修改请求
// - ErrorPacket: 错误的请求
// - EOFPacket: 在 Result Set 中使用
//
// # 在 MySQL 中作为一个请求-响应协议以如下方式使用
//
// - 客户端发送 SQL 语句到服务端
// - 服务器端根据 SQL 响应不同的数据包 以 OkPacket 举例
//
// +--------------------+                      +-----------------+
// |     Client         |                      |      Server     |
// +--------------------+                      +-----------------+
// | N                  |  ---------------->   |                 |
// | INSERT users       |                      |                 |
// | (username,age)     |                      |                 |
// | VALUES('john',30); |                      |                 |
// +--------------------+                      +-----------------+
// |                    |  <----------------   | +OkPacket       |
// |                    |                      | 1 row affected  |
// +--------------------+                      +-----------------+
//
// 从 MySQL Connection 的视角来看 会区分为两个 Stream 以及初始化两个不同的 decoder
// 分别负责 MySQL/request 和 MySQL/response 的解析 如果解析成功后会向上层提交一个 *roundtrip.Object
// 上层拿到 *roundtrip.Object 会进行请求的配对 成功则代表成功捕获到一次 MySQL 请求
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

		// Request 请求一旦完成即可归档
		if d.role == role.Request {
			return d.archive(), nil
		}
		// Response 请求需要保证所有数据均被消费完
		// maxPayloadSize 表示后续还有数据包
		if len(b) == 0 && d.payloadLen != maxPayloadSize && d.payloadConsumed == d.payloadLen {
			if d.obj == nil {
				return nil, err
			}
			switch d.obj.(type) {
			case *OKPacket:
				return d.archive(), nil
			case *ErrorPacket:
				return d.archive(), nil
			default:
				// *EOFPacket
				// *ResultSetPacket
				if d.eofPackets != 2 {
					return nil, nil
				}
				return d.archive(), nil
			}
		}
	}

	return nil, nil
}

// Free 释放持有的资源
func (d *decoder) Free() {
	d.statement = nil
}

var ignoreCmdStatement = map[uint8]struct{}{
	cmdProcess:  {},
	cmdDebug:    {},
	cmdCreateDB: {},
}

// normalizeStatement 格式化执行语句
func (d *decoder) normalizeStatement() string {
	_, ok := ignoreCmdStatement[d.cmdType]
	if ok {
		return ""
	}
	cloned := d.statement.Clone()
	b := bytes.ReplaceAll(cloned, splitio.CharLF, []byte(" "))
	return string(b)
}

// archive 归档请求
func (d *decoder) archive() []*role.Object {
	if d.role == role.Request {
		obj := role.NewRequestObject(&Request{
			Host:      d.st.SrcIP,
			Port:      d.st.SrcPort,
			Proto:     PROTO,
			Command:   commands[d.cmdType],
			Statement: d.normalizeStatement(), // 内存拷贝
			Size:      d.drainBytes,
			Time:      d.reqTime,
		})
		d.reset()
		return []*role.Object{obj}
	}

	var packet any
	switch v := d.obj.(type) {
	case *OKPacket:
		packet = v
	case *ErrorPacket:
		packet = v
	case *EOFPacket:
		packet = &ResultSetPacket{Rows: d.headers - 1} // 减去最后一个 EOFPacket
	}

	obj := role.NewResponseObject(&Response{
		Host:   d.st.SrcIP,
		Port:   d.st.SrcPort,
		Proto:  PROTO,
		Size:   d.drainBytes,
		Packet: packet,
		Time:   d.t0,
	})
	d.reset()
	return []*role.Object{obj}
}

// decode 真正的解析入口
func (d *decoder) decode(b []byte) ([]byte, bool, error) {
	if d.state == stateDecodeHeader {
		if len(b) < headerLength {
			d.partial++
			d.tail = bytes.Clone(b)
			return nil, false, errDecodeHeader
		}
		if err := d.decodeHeader(b[:headerLength]); err != nil {
			return nil, false, err
		}
		b = b[headerLength:]
		d.drainBytes += headerLength
	}

	if len(b) == 0 {
		return nil, false, nil
	}
	return d.decodePayload(b)
}

// decodeHeader 解析协议 Header 固定 4 字节
//
// +--------------------------------------+-------------------+
// |  Payload Length (3B)                 |  Sequence ID (1B) |
// +--------------------------------------+-------------------+
//
// Payload Length: 包体的长度（小端字节序，最大 16MB-1）
// Sequence ID: 包序列号（从 0 开始 每发送一次递增 响应包复用相同序列号）
func (d *decoder) decodeHeader(b []byte) error {
	n := decode3ByteN(b)
	if n > maxPayloadSize || n < 0 {
		return errDecodeHeader
	}

	d.payloadLen = uint32(n)
	d.seqID = b[3]
	d.state = stateDecodePayload
	d.payloadConsumed = 0
	d.headers++
	return nil
}

func (d *decoder) isClient() bool {
	return uint16(d.serverPort) == d.st.DstPort
}

func (d *decoder) guessRequest(b byte) bool {
	if !d.isClient() {
		return false
	}
	if d.seqID == 0 {
		return true
	}
	if _, ok := commands[b]; ok && d.payloadLen >= 6 {
		return true
	}
	return false
}

// decodePayload 解析协议 Payload 不定长度
//
// Payload 可能包含【多种】类型的数据包 需要按 Length-Payload 的顺序交替解析
// 单次 TCP 包可能包含【多个】数据包
func (d *decoder) decodePayload(b []byte) ([]byte, bool, error) {
	if d.role == role.Request {
		complete, err := d.decodeRequest(b)
		return nil, complete, err
	}

	// 匹配到 cmdType 则确认为 Request
	if d.isClient() {
		d.role = ""
	}
	if d.role == "" && d.guessRequest(b[0]) {
		d.state = stateDecodePayload
		d.role = role.Request
		d.cmdType = b[0]
		d.reqTime = d.t0
		d.payloadConsumed++
		d.drainBytes++
		complete, err := d.decodeRequest(b[1:])
		return nil, complete, err
	}

	// 优先解析剩余 Response 内容
	if d.waitForRsp {
		d.waitForRsp = false
		if d.payloadConsumed > d.payloadLen {
			return nil, false, errDecodeResponse
		}
		n := d.payloadLen - d.payloadConsumed
		if len(b) <= int(n) {
			b, complete, err := d.decodeResponse(b)
			return b, complete, err
		}
		_, _, _ = d.decodeResponse(b[:n]) // 必定解析成功 忽略异常
		return b[n:], true, nil
	}

	// 根据首字节判断数据包类型
	switch b[0] {
	case packetEOF:
		d.packetType = packetEOF
		d.eofPackets++
		d.payloadConsumed++
		d.drainBytes++

		b, obj, err := d.decodeEOFPacket(b[1:])
		if err != nil {
			return nil, false, err
		}
		d.obj = obj

		// 结果集最多有两个 EOFPacket
		if d.eofPackets == 2 {
			return nil, true, nil
		}
		d.headers = 0 // 首次解析数据行
		return d.decodeResponse(b)

	case packetError:
		d.packetType = packetError
		d.payloadConsumed++
		d.drainBytes++

		b, obj, err := d.decodeErrPacket(b[1:])
		if err != nil {
			return nil, false, err
		}
		d.obj = obj
		return d.decodeResponse(b)

	case packetOK:
		d.packetType = packetOK
		d.payloadConsumed++
		d.drainBytes++

		b, obj, err := d.decodeOkPacket(b[1:])
		if err != nil {
			return nil, false, err
		}
		d.obj = obj
		b, complete, err := d.decodeResponse(b)
		return b, complete, nil

		// AuthSwitch / packetLocalInfile 两种数据包不做处理
		// case packetAuthSwitch, packetLocalInfile:
		//	return nil, false, nil
	}

	return d.decodeResponse(b)
}

type ResultSetPacket struct {
	Rows int
}

func (p ResultSetPacket) Name() string {
	return "ResultSet"
}

// decodeResponse 解析 Response 数据 查询请求的话得分多轮解析
//
// # Client                      # Server
// |                               |
// | --------- COM_QUERY --------- |
// |                               |
// |      ← Result Set Columns     |
// |      ← Column Definition 1    |
// |      ← ...                    |
// |      ← Column Definition N    |
// |      ← EOF Packet             |
// |      ← Row Data 1             |
// |      ← ...                    |
// |      ← Row Data N             |
// |      ← EOF Packet             |
// | ----------------------------- |
//
// `Result Set Packet`: 查询结果集
// 当执行 SELECT 或 SHOW 等查询时 服务器返回 `Result Set Packet` 包含以下部分
//
// 1) Column Count Packet: 列数量包
// 长度可变数值（Length-Encoded Integer）表示结果集的列数
//
// 2) Column Definition Packet: 列定义包
// 每列一个数据包 总数为列数量 内容为列名、类型、长度、字符集等信息
//
// 3) End of Field: EOF 包 标识列定义包的结束 固定数值 0xFE
//
// 4) Row Data Packet: 行数据包
// 每行一个数据包 直到遇到 EOF 包 每个字段为 长度编码字符串（Length-Encoded String)
//
// 5）End of Packet: 最终 EOF 包 标识结果集传输完成 固定数值 0xFE
func (d *decoder) decodeResponse(b []byte) ([]byte, bool, error) {
	// 解析 payloadLen 时出现异常
	if d.payloadConsumed > d.payloadLen {
		return nil, false, errDecodeResponse
	}

	d.role = role.Response
	n := d.payloadConsumed + uint32(len(b))

	// 刚好能够拼接成一个数据包
	if n == d.payloadLen {
		d.payloadConsumed += uint32(len(b))
		d.drainBytes += len(b)
		d.state = stateDecodeHeader
		return nil, true, nil
	}

	// 剩余的数据已经超过一个完整的 payload 所需要的字节
	// 则需要消费剩下的内容并将 tail 返回
	if n > d.payloadLen {
		consumed := d.payloadLen - d.payloadConsumed
		d.payloadConsumed += consumed
		d.drainBytes += int(consumed)
		d.state = stateDecodeHeader
		return b[consumed:], true, nil
	}

	// 进入此逻辑则下一轮将继续解析剩下的 Response
	d.waitForRsp = true
	d.drainBytes += int(n)
	d.payloadConsumed += n
	return nil, false, nil
}

// decodeRequest 解析 Request 数据
// TODO(mando): 是否应该解析 MySQL 执行语句？评估性能以及要上报什么字段
func (d *decoder) decodeRequest(b []byte) (bool, error) {
	d.payloadConsumed += uint32(len(b))
	d.drainBytes += len(b)

	d.statement.Write(b)
	if d.payloadConsumed > d.payloadLen {
		return false, errInvalidBytes
	}
	if d.payloadConsumed == d.payloadLen {
		return true, nil
	}
	return false, nil
}

type EOFPacket struct {
	StatusFlags int
	Warnings    int
}

func (p EOFPacket) Name() string {
	return "EOF"
}

// decodeEOFPacket 解编码 EOFPacket
//
// MySQL EOF Packet Format (Legacy)
// ┌───────────┬───────────────┬───────────────┐
// │ Identifier│ Status Flags  │ Warning Count │
// │    (1B)   │    (2B LE)    │    (2B LE)    │
// ├───────────┼───────────────┼───────────────┤
// │   0xFE    │   0x0000      │    0x0001     │
// └───────────┴───────────────┴───────────────┘
//
// * MySQL 5.7.5+
// 如果客户端启用了 CLIENT_DEPRECATE_EOF 标志 则 EOF Packet 被替换为 OK Packet
// 需解析 OK Packet 格式（标识符 0x00 或 0xFE 后跟更多字段）
func (d *decoder) decodeEOFPacket(b []byte) ([]byte, *EOFPacket, error) {
	if len(b) < 4 {
		d.partial++
		d.tail = bytes.Clone(b)
		return nil, nil, errDecodeEOFPacket
	}

	eofPacket := &EOFPacket{
		StatusFlags: decode2ByteN(b[:2]),
		Warnings:    decode2ByteN(b[2:4]),
	}
	d.payloadConsumed += 4
	d.drainBytes += 4
	return b[4:], eofPacket, nil
}

type ErrorPacket struct {
	ErrCode  int
	ErrMsg   string
	SQLState string
}

func (p ErrorPacket) Name() string {
	return "Error"
}

// decodeErrPacket 解编码 ErrorPacket
//
// MySQL Error Packet Format
// ┌───────────┬──────────────┬──────────────────┬───────────────────┐
// │ Identifier│ Error Code   │ SQL State Marker │ Error Message     │
// │    (1B)   │   (2B LE)    │ + SQL State (6B) │ (NUL-terminated)  │
// ├───────────┼──────────────┼──────────────────┼───────────────────┤
// │   0xFF    │   0x0426     │     #23000       │  "Duplicate key"  │
// └───────────┴──────────────┴──────────────────┴───────────────────┘
//
// 这里仅解析 ErrorCode 和 SQLState 后者去掉了 Marker
func (d *decoder) decodeErrPacket(b []byte) ([]byte, *ErrorPacket, error) {
	if len(b) < 8 {
		d.partial++
		d.tail = bytes.Clone(b)
		return nil, nil, errDecodeErrPacket
	}

	code := decode2ByteN(b)
	eofPacket := &ErrorPacket{
		ErrCode:  code,
		SQLState: string(b[3:8]),
	}

	b = b[8:]

	var n int
	if idx := bytes.IndexByte(b, splitio.CharLF[0]); idx > 0 {
		n = min(idx, maxErrMsgSize)
	} else {
		n = min(len(b), maxErrMsgSize)
	}
	eofPacket.ErrMsg = string(b[:n])

	d.payloadConsumed += uint32(8 + n)
	d.drainBytes += 8 + n
	return b[n:], eofPacket, nil
}

type OKPacket struct {
	AffectedRows int
	LastInsertID int
	Status       int
	Warnings     int
}

func (p OKPacket) Name() string {
	return "OK"
}

// decodeOkPacket 解编码 OkPacket
//
// MySQL OK Packet Format
// ┌───────────┬─────────────────┬─────────────────┬─────────────┬─────────────┬───────────────────┐
// │ Identifier│ Affected Rows   │ Last Insert ID  │ Status Flags│  Warnings   │ Session State     │
// │    (1B)   │ (Length-Encoded)│ (Length-Encoded)│   (2B LE)   │   (2B LE)   │ (Optional, LE Str)│
// ├───────────┼─────────────────┼─────────────────┼─────────────┼─────────────┼───────────────────┤
// │   0x00    │      0x01       │       0x05      │    0x0A00   │    0x0000   │         -         │
// └───────────┴─────────────────┴─────────────────┴─────────────┴─────────────┴───────────────────┘
//
// * LE (Length-Encoded Integer): 可变长度整数编码
// * 所有数值类型均为小端编码
func (d *decoder) decodeOkPacket(b []byte) ([]byte, *OKPacket, error) {
	if len(b) < 6 {
		d.partial++
		d.tail = bytes.Clone(b)
		return nil, nil, errDecodeOKPacket
	}

	prevLen := len(b)
	affectedRows, b, ok := decodeLenEncodedInteger(b)
	if !ok {
		return nil, nil, errDecodeOKPacket
	}
	lastInsertID, b, ok := decodeLenEncodedInteger(b)
	if !ok {
		return nil, nil, errDecodeOKPacket
	}
	status, b, ok := decodeLenEncodedInteger(b)
	if !ok {
		return nil, nil, errDecodeOKPacket
	}
	warnings, b, ok := decodeLenEncodedInteger(b)
	if !ok {
		return nil, nil, errDecodeOKPacket
	}

	d.payloadConsumed += uint32(prevLen - len(b))
	d.drainBytes += prevLen - len(b)
	okPacket := &OKPacket{
		AffectedRows: affectedRows,
		LastInsertID: lastInsertID,
		Status:       status,
		Warnings:     warnings,
	}
	return b, okPacket, nil
}

// decodeLenEncodedInteger 解编码可变长度数值
//
// MySQL 通信协议里 0xfb、0xfd、0xfd、0xfe 用于可变长度数值（Length-Encoded Integer）
// 可变长度数值的编码规则取决于首字节的值 不同的值表示后续数据的不同长度
//
// * 0xfb: NULL
// * 0xfc: 小端序 2 字节编码
// * 0xfd: 小端序 3 字节编码
// * 0xfe: 小端序 8 字节编码
//
// 函数返回解析后的数值 以及未 decode 的字节
func decodeLenEncodedInteger(data []byte) (int, []byte, bool) {
	if len(data) == 0 {
		return 0, nil, false
	}

	switch data[0] {
	case 0xfb:
		return 0, data[1:], true

	case 0xfc:
		if len(data) < 3 {
			return 0, data, false
		}
		n := decode2ByteN(data[1:3])
		return n, data[3:], true

	case 0xfd:
		if len(data) < 4 {
			return 0, data, false
		}
		n := decode3ByteN(data[1:4])
		return n, data[4:], true

	case 0xfe:
		if len(data) < 9 {
			return 0, data, false
		}
		n := decode8ByteN(data[1:9])
		return n, data[9:], true
	}

	return int(data[0]), data[1:], true
}

func decode2ByteN(b []byte) int {
	return int(binary.LittleEndian.Uint16(b[:2]))
}

func decode3ByteN(b []byte) int {
	return int(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16)
}

func decode8ByteN(b []byte) int {
	return int(binary.LittleEndian.Uint64(b[:8]))
}
