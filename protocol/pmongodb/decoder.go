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

package pmongodb

import (
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
	PROTO = "MongoDB"
)

func newError(format string, args ...any) error {
	format = "mongodb/decoder: " + format
	return errors.Errorf(format, args...)
}

var (
	errDecodeInt32  = newError("decode int32 bytes failed")
	errDecodeHeader = newError("decode header failed")
)

const (
	// headerLength header 固定长度
	headerLength = 16

	// maxPayloadSize 单 payload 最大长度 超过此长度服务端会进行切片
	maxPayloadSize = 0xFFFFFF
)

const (
	// bsonStringType bson.String 类型标识
	bsonStringType = 0x02

	// bsonStringEnd bson.String 结束标识
	// C 语言风格字符串
	bsonStringEnd = 0x00

	// bsonGapKeyValue bson.String Key/Value 相隔字节数
	bsonGapKeyValue = 5

	// bsonDoubleType bson.Double 类型标识
	bsonDoubleType = 0x01

	// bsonInt32Type bson.Int32 类型标识
	bsonInt32Type = 0x10

	// bsonInt64Type bson.Int64 类型标识
	bsonInt64Type = 0x12

	// bsonBodySection body section 起始标识
	bsonBodySection = 0x00
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
	OptEnableResponseCode = "enableResponseCode"
)

type decoder struct {
	st    socket.TupleRaw
	t0    time.Time
	state state

	msgHdr    *msgHeader
	sourceCmd sourceCommand
	okCode    okCode
	reqTime   time.Time

	payloadConsumed       int
	bodySectionSize       int
	bodySectionDrainBytes int

	enableRspCode bool
}

func NewDecoder(st socket.Tuple, _ socket.Port, opts common.Options) protocol.Decoder {
	enableRspCode, _ := opts.GetBool(OptEnableResponseCode)
	return &decoder{
		st:            st.ToRaw(),
		enableRspCode: enableRspCode,
	}
}

// reset 重置单次请求状态
func (d *decoder) reset() {
	d.state = stateDecodeHeader
	d.payloadConsumed = 0
	d.bodySectionSize = 0
	d.bodySectionDrainBytes = 0
	d.sourceCmd = sourceCommand{}
	d.msgHdr = nil
}

// Free 释放持有的资源
func (d *decoder) Free() {
	d.msgHdr = nil
}

// Decode 持续从 zerocopy.Reader 解析 MongoDB 协议数据流，构建并返回 RoundTrip 对象
//
// # Decode 要求具备容错和自恢复能力 即当出现错误的时候能够适当重置
//
// Section 部分仅解析 OP_MSG 数据类型 提取 DB/Collection/Command 等信息
// 在现代的 MongoDB 版本（v3.6+) 中已经使用 OP_MSG 替换之前的各种 OP
// 对于分片的 Body Section 需要支持连续解析能力 尽可能保证元数据信息的完整性
//
// # 在 MongoDB 中作为一个请求-响应协议以如下方式使用
//
// +--------------------+                      +-----------------+
// |     Client         |                      |      Server     |
// +--------------------+                      +-----------------+
// | OP_MSG             |  ---------------->   |                 |
// | { insert: "users", |                      |                 |
// |   documents: [...]}|                      |                 |
// +--------------------+                      +-----------------+
// |                    |  <----------------   | OP_MSG Response |
// |                    |                      | { ok: 1, ... }  |
// +--------------------+                      +-----------------+
//
// 从 MongoDB Connection 的视角来看 会区分为两个 Stream 以及初始化两个不同的 decoder
// 分别负责 MongoDB/request 和 MongoDB/response 的解析 如果解析成功后会向上层提交一个 *roundtrip.Object
// 构建 RoundTrip 需要将 RequestID 和 ResponseTO 进行配对 成功则代表成功捕获到一次 MongoDB 请求
//
// 为了尽量模拟近似的 `请求时间`
// Request.Time 从发送的第一个数据包开始计时
// Response.Time 从接收的最后一个数据包停止计时
func (d *decoder) Decode(r zerocopy.Reader, t time.Time) ([]*role.Object, error) {
	d.t0 = t

	b, err := r.Read(common.ReadWriteBlockSize)
	if err != nil {
		d.reset()
		return nil, err
	}

	obj, err := d.decode(b)
	if err != nil {
		d.reset()
		return nil, err
	}

	if obj == nil {
		return nil, nil
	}
	return []*role.Object{obj}, nil
}

// decode 真正的解析入口
func (d *decoder) decode(b []byte) (*role.Object, error) {
	if d.state == stateDecodeHeader {
		if len(b) < headerLength {
			return nil, errDecodeHeader
		}

		msgHdr, err := d.decodeHeader(b[:headerLength])
		if err != nil {
			return nil, err
		}

		if msgHdr.isRequest() {
			d.reqTime = d.t0
		}

		d.payloadConsumed += headerLength
		d.msgHdr = msgHdr
		d.state = stateDecodePayload
		b = b[headerLength:]
	}

	complete := d.decodePayload(b)
	if !complete {
		return nil, nil
	}
	return d.archive(), nil
}

// archive 归档请求
func (d *decoder) archive() *role.Object {
	defer d.reset()

	if d.msgHdr.isRequest() {
		if d.sourceCmd.IsEmpty() {
			return nil
		}

		obj := role.NewRequestObject(&Request{
			Host:       d.st.SrcIP,
			Port:       d.st.SrcPort,
			ID:         d.msgHdr.reqID,
			Proto:      PROTO,
			OpCode:     opcodes[opcode(d.msgHdr.opCode)],
			Source:     d.sourceCmd.source,
			Collection: d.sourceCmd.collection,
			CmdName:    d.sourceCmd.cmdName,
			CmdValue:   d.sourceCmd.cmdValue,
			Size:       d.payloadConsumed,
			Time:       d.reqTime,
		})
		return obj
	}

	obj := role.NewResponseObject(&Response{
		Host:    d.st.SrcIP,
		Port:    d.st.SrcPort,
		ID:      d.msgHdr.rspTo,
		Proto:   PROTO,
		OpCode:  opcodes[opcode(d.msgHdr.opCode)],
		Ok:      d.okCode.ok,
		Code:    d.okCode.code,
		Message: codeMessages[d.okCode.code],
		Size:    d.payloadConsumed,
		Time:    d.t0,
	})
	return obj
}

type msgHeader struct {
	length int32
	reqID  int32
	rspTo  int32
	opCode int32
}

func (h msgHeader) isRequest() bool {
	return h.reqID > 0 && h.rspTo == 0
}

func (h msgHeader) isValid() bool {
	if h.length > maxPayloadSize || h.length < 0 {
		return false
	}
	_, ok := opcodes[opcode(h.opCode)]
	return ok
}

// decodeHeader 解析 MongoDB header 部分 数据布局如下
//
// ┌─────────────────────── 16 Bytes Header ───────────────────────┐
// │                                                               │
// │ 0                   1                   2                   3 │
// │ 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │          messageLength        │         requestId             │
// ├───────┬───────┬───────┬───────┼───────┬───────┬───────┬───────┤
// │          responseTo           │         opcode                │
// └───────┴───────┴───────┴───────┴───────┴───────┴───────┴───────┘
//
// - messageLength: 消息总长度（包括 Header 和 Body）
// - requestId: 客户端生成的唯一请求标识符 用于匹配请求与响应
// - responseTo: 服务器响应时回填的原始请求 ID（请求包中固定为 0）
// - opcode: 操作类型标识符（如 OP_MSG=2013、OP_REPLY=1）
func (d *decoder) decodeHeader(b []byte) (*msgHeader, error) {
	if len(b) < headerLength {
		return nil, errDecodeHeader
	}

	length, err := decodeInt32(b[:4])
	if err != nil {
		return nil, err
	}
	reqID, err := decodeInt32(b[4:8])
	if err != nil {
		return nil, err
	}
	rspTo, err := decodeInt32(b[8:12])
	if err != nil {
		return nil, err
	}
	opCode, err := decodeInt32(b[12:16])
	if err != nil {
		return nil, err
	}

	hdr := &msgHeader{
		length: length,
		reqID:  reqID,
		rspTo:  rspTo,
		opCode: opCode,
	}
	if !hdr.isValid() {
		return nil, errDecodeHeader
	}
	return hdr, nil
}

// decodeBodySection 解析 BodySection
//
// OP_MSG Body:
// ┌───────┬───────┬───────┬───────┐
// │ 0x01 0x00 0x00 0x00           │ → Flags=0x1 (Checksum enabled)
// ├───────┬───────────────────────┤
// │ 0x00  │ 0x25 0x00 0x00 0x00   │ → Section Type 0 + Doc length=37 bytes
// │       │ { insert: "users",    │
// │       │   ordered: true,      │
// │       │   documents: [...] }  │
// ├───────┬──────────────┬────────┤
// │ 0x01  │ "documents\x00"       │ → Section Type 1 + Identifier
// │       │ 0x15 0x00 0x00 0x00   │ → Doc length=21 bytes
// │       │ { _id: 1, name: "A" } │
// ├───────┬───────┬───────┬───────┤
// │ 0xEF 0xCD 0xAB 0x89           │ → Checksum=0x89ABCDEF
// └───────┴───────┴───────┴───────┘
//
// Body Section 是 OP_MSG 消息的核心部分 负责承载数据库操作的实际数据和控制信息
// 这里主要是为了提取操作的 DB/Collection 以及 Command
func (d *decoder) decodeBodySection(b []byte) {
	if len(b) < 10 {
		return
	}

	// 如果已经处理完 body section 则不再尝试
	if d.bodySectionDrainBytes > d.bodySectionSize {
		return
	}
	// 已经解析到 sourceCmd 也不再尝试
	if d.sourceCmd.source != "" && d.sourceCmd.cmdName != "" {
		return
	}

	var l, r int
	// 只在第一次的解析 Body Section 的时候解析 Length
	// 当 TCP 分片的时候可以接着解析
	if d.bodySectionSize == 0 {
		// 取第 5 字节
		// 前 4 字节为 Flags 字段
		if b[4] != bsonBodySection {
			return
		}

		// 解析 body section 的长度
		n, err := decodeInt32(b[bsonGapKeyValue : bsonGapKeyValue+4])
		if err != nil {
			return
		}
		if n > maxPayloadSize {
			return
		}

		d.bodySectionSize = int(n)
		l = bsonGapKeyValue
		r = int(n) + bsonGapKeyValue
	}

	// 确保不会索引越界
	if r > len(b) || r == 0 {
		r = len(b)
	}
	d.bodySectionSize += r - l // 记录已经消费的 body section 长度

	if d.msgHdr.isRequest() {
		sc := decodeSourceCommand(b[l:r])
		if sc.source != "" {
			d.sourceCmd.source = sc.source
		}
		if sc.collection != "" {
			d.sourceCmd.collection = sc.collection
		}
		if sc.cmdName != "" && sc.cmdValue != "" {
			d.sourceCmd.cmdName = sc.cmdName
			d.sourceCmd.cmdValue = sc.cmdValue
		}
		return
	}

	// 解析 Response Code/Ok 会带来大量的 CPU 开销
	// 建议按需启用（默认不开启）
	if d.enableRspCode {
		oc := decodeOkCode(b[l:r])
		d.okCode.ok = oc.ok
		d.okCode.code = oc.code
	}
}

// decodePayload 解析 payload 布局如下 以 OP_MSG 为例
//
// ┌───────────────────────── OP_MSG Body ─────────────────────────┐
// │ 0                   1                   2                   3 │
// │ 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │                            Flags (4 Bytes)                    │
// ├───────┬───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │  Type │  Section Data (Variable length, depends on type)      │
// │ (0/1) │                                                       │
// ├─ ... ─┴───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │  Type │  Section Data                                         │
// │ (0/1) │                                                       │
// ├─ ... ─┴───────┬───────┬───────┬───────┬───────┬───────┬───────┤
// │                        Checksum (Optional, 4 Bytes)           │
// └───────┴───────┴───────┴───────┴───────┴───────┴───────┴───────┘
//
// 其中第一个 Section Flag 为 0x00 的 Body Section
// 紧接着的所有 Section 为 Document Section
func (d *decoder) decodePayload(b []byte) bool {
	// 这里仅支持 OP_MSG
	// 从 MongoDB 3.6 开始 OP_MSG 支持传输任意 BSON 数据
	// OP_QUERY 不再做兼容支持
	if opcode(d.msgHdr.opCode) == opcodeMsg {
		d.decodeBodySection(b)
	}

	n := d.payloadConsumed + len(b)
	if n == int(d.msgHdr.length) {
		d.payloadConsumed += len(b)
		return true
	}

	if n > int(d.msgHdr.length) {
		consumed := int(d.msgHdr.length) - d.payloadConsumed
		d.payloadConsumed += consumed
		return true
	}

	d.payloadConsumed += len(b)
	return false
}

func decodeInt32(b []byte) (int32, error) {
	if len(b) < 4 {
		return 0, errDecodeInt32
	}

	n := int64(binary.LittleEndian.Uint32(b))
	if n > math.MaxInt32 {
		return 0, errDecodeInt32
	}
	return int32(n), nil
}

type sourceCommand struct {
	source     string
	collection string
	cmdName    string
	cmdValue   string
}

func (sc sourceCommand) IsEmpty() bool {
	var emtpy sourceCommand
	return emtpy == sc
}

// decodeSourceCommand 解析 SourceCommand
//
// 即解析当次 MongoDB 请求的 DB/Collection 以及执行的 Command
// 需要先通过切割 Bson 中关键类型的位置再逐一进行解析
func decodeSourceCommand(b []byte) sourceCommand {
	type kv struct {
		k, v string
	}

	var cmd *kv
	var source string
	var collection string

	splitStringBsonTypePos(b, func(typePos bsonTypePositions) bool {
		key := string(b[typePos[0].pos+1 : typePos[1].pos])
		if cmd == nil && isCommand(key) {
			val := string(b[typePos[1].pos+bsonGapKeyValue : typePos[2].pos])
			cmd = &kv{
				k: key,
				v: val,
			}

			if isCommandWithCollection(key) {
				collection = val
			}
		}

		if collection == "" && key == "collection" {
			collection = string(b[typePos[1].pos+bsonGapKeyValue : typePos[2].pos])
		}

		if source == "" {
			switch key {
			case "$db", "ns":
				// [ns] key:
				// MongoDB 的系统集合（如 oplog.rs 或 system.profile）中的文档会包含 ns 字段
				// 表示该操作作用的命名空间（Namespace 格式为 <database>.<collection>）
				//
				// [$db] key:
				// 在 OP_MSG 中为必选字段 声明操作的 db
				source = string(b[typePos[1].pos+bsonGapKeyValue : typePos[2].pos])
			}
		}

		// 完成解析则不再继续尝试
		// cmd 可能到最后也无法解析
		// 如 hello 命令 其响应是数值类型 {"hello": 1}
		if source != "" && cmd != nil {
			return true
		}
		return false
	})

	sc := sourceCommand{
		collection: collection,
		source:     source,
	}

	if cmd != nil {
		sc.cmdName = cmd.k
		sc.cmdValue = cmd.v
		return sc
	}

	var cmdVal float64
	var cmdName string

	// 部分命令是数值类型的响应 再这里再次尝试解析 如
	// {"hello": 1}
	// {"listDatabases": 1}
	// {"getMore": 1000}
	splitIntOrDoubleBsonTypePos(b, func(typePos bsonTypePositions) bool {
		key := string(b[typePos[0].pos+1 : typePos[1].pos])
		if !isCommand(key) {
			return false
		}

		l := typePos[1].pos + 1
		if len(b) < l+1 {
			return false
		}

		switch typePos[0].typ {
		case bsonInt32Type:
			r := l + 4 // 4 字节小端整型
			if len(b) < r {
				return false
			}
			cmdVal = float64(binary.LittleEndian.Uint32(b[l:r]))

		case bsonInt64Type:
			r := l + 8 // 8 字节小端整型
			if len(b) < r {
				return false
			}
			cmdVal = float64(binary.LittleEndian.Uint64(b[l:r]))

		case bsonDoubleType:
			r := l + 8 // 8 字节 IEEE754 浮点数
			if len(b) < r {
				return false
			}
			cmdVal = math.Float64frombits(binary.LittleEndian.Uint64(b[l:r]))
		}

		cmdName = key
		return true
	})

	if cmdName != "" {
		sc.cmdName = cmdName
		sc.cmdValue = strconv.Itoa(int(cmdVal))
	}
	return sc
}

type okCode struct {
	ok   float64
	code int32
}

func decodeOkCode(b []byte) okCode {
	var ok float64 = -1
	var code int32 = -1

	const (
		okKey   = "ok"
		codeKey = "code"
	)

	splitIntOrDoubleBsonTypePos(b, func(typePos bsonTypePositions) bool {
		key := string(b[typePos[0].pos+1 : typePos[1].pos])

		l := typePos[1].pos + 1
		if len(b) < l+1 {
			return false
		}
		switch key {
		case okKey:
			if ok != -1 {
				return false
			}
			r := l + 8 // 8 字节 IEEE754 浮点数
			if len(b) < r {
				return false
			}
			ok = math.Float64frombits(binary.LittleEndian.Uint64(b[l:r]))

		case codeKey:
			if code != -1 {
				return false
			}
			r := l + 4 // 4 字节小端整型
			if len(b) < r {
				return false
			}
			code = int32(binary.LittleEndian.Uint32(b[l:r]))
		}

		if ok == 1 {
			return true
		}
		if ok == 0 && code != -1 {
			return true
		}
		return false
	})

	oc := okCode{}
	if ok > 0 {
		oc.ok = ok
	}
	if code > 0 {
		oc.code = code
	}
	return oc
}

type typePosition struct {
	typ uint8
	pos int
}

type bsonTypePositions []typePosition

// splitStringBsonTypePos 分割 bson 数据 每个 bsonTypePos 是长度为 3 的数组
//
// f 会在每个 bsonTypePos 生成时调用 返回值表示是否结束流程
//
// [0] bsonStringType Pos: KeyStart
// [1] bsonStringEnd Pos: KeyEnd
// [2] bsonStringEnd Pos: ValueEnd
//
// 元素要求 pos 必须为递增且均能被正确取值到 不会出现 [out of index]
// 只切割 String 类型的 bson 数据
//
// 实际上这是一个折中的方案 要求 `快速` 识别请求的 Command 和 DB/Collection
// 这两者一定是 String 类型 且不会存在嵌套解析的逻辑 所以快速切割出 StringField 是个最优解
// 代价是可能不会 100% 准确 当遇到 TCP 包切割的时候不会自动拼接前后两个数据包
// 但考虑到一个 String Field 刚好被切割在两个不同的 TCP 包（且这两个 Field 是 Command/DB/Collection）
// 的概率是相对较小的 所以此方案是可接受的
func splitStringBsonTypePos(b []byte, f func(bsonTypePositions) bool) {
	var typePositions bsonTypePositions
	var cursor int

	length := len(b)
	for cursor < length {
		switch b[cursor] {
		case bsonStringType:
			if len(typePositions) != 0 {
				typePositions = bsonTypePositions{}
			}
			typePositions = append(typePositions, typePosition{
				typ: bsonStringType,
				pos: cursor,
			})

		case bsonStringEnd:
			switch len(typePositions) {
			case 1:
				typePositions = append(typePositions, typePosition{
					typ: bsonStringEnd,
					pos: cursor,
				})
				cursor += 4 // skip length

			case 2:
				// 确保 typePositions 组合是有意义的
				// 因为这里可能出现 `异常的` 相连的 End 标识
				if typePositions[1].pos+bsonGapKeyValue > cursor {
					typePositions = bsonTypePositions{}
					cursor++
					continue // 置空并从头开始
				}
				typePositions = append(typePositions, typePosition{
					typ: bsonStringEnd,
					pos: cursor,
				})

				quit := f(typePositions)
				if quit {
					return
				}
				typePositions = bsonTypePositions{}
			}
		}
		cursor++
	}
}

// splitIntOrDoubleBsonTypePos 分割 bson 数据 每个 bsonTypePos 是长度为 2 的数组
//
// f 会在每个 bsonTypePos 生成时调用 返回值表示是否结束流程
//
// [0] bsonInt32Type / bsonDoubleType Pos: Type
// [1] bsonStringEnd Pos: KeyEnd
//
// 只切割 bsonInt32Type / bsonDoubleType 类型的 bson 数据
func splitIntOrDoubleBsonTypePos(b []byte, f func(bsonTypePositions) bool) {
	var typePositions bsonTypePositions
	var cursor int

	length := len(b)
	for cursor < length {
		switch b[cursor] {
		case bsonStringEnd:
			// 必须为第二个元素
			if len(typePositions) != 1 {
				typePositions = bsonTypePositions{}
				cursor++
				continue
			}
			typePositions = append(typePositions, typePosition{
				typ: b[cursor],
				pos: cursor,
			})
			quit := f(typePositions)
			if quit {
				return
			}
			typePositions = bsonTypePositions{}

		case bsonInt32Type:
			// 必须为第一个元素
			if len(typePositions) != 0 {
				typePositions = bsonTypePositions{}
			}
			typePositions = append(typePositions, typePosition{
				typ: bsonInt32Type,
				pos: cursor,
			})

		case bsonInt64Type:
			// 必须为第一个元素
			if len(typePositions) != 0 {
				typePositions = bsonTypePositions{}
			}
			typePositions = append(typePositions, typePosition{
				typ: bsonInt64Type,
				pos: cursor,
			})

		case bsonDoubleType:
			// 必须为第一个元素
			if len(typePositions) != 0 {
				typePositions = bsonTypePositions{}
			}
			typePositions = append(typePositions, typePosition{
				typ: bsonDoubleType,
				pos: cursor,
			})
		}
		cursor++
	}
}
