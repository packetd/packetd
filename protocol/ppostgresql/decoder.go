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

package ppostgresql

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/bufbytes"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

const (
	PROTO = "PostgreSQL"
)

func newError(format string, args ...any) error {
	format = "postgresql/decoder: " + format
	return errors.Errorf(format, args...)
}

var (
	errInvalidBytes = newError("invalid bytes")
	errDecodeHeader = newError("decode Header failed")
)

const (
	// headerLength header 固定字节长度
	headerLength = 5

	// maxStatementSize SQL 语句缓冲区大小
	maxStatementSize = 1024

	// maxStatementNameSize statement 名称缓冲区大小
	maxStatementNameSize = 64

	// maxDescribeSize describe 缓冲区大小
	maxDescribeSize = 64

	// cStringEnd c 字符串结束标识
	cStringEnd byte = '\x00'

	// startupMessage startup message 标识
	startupMessage uint32 = 196608

	// maxNamedCacheSize named cache 缓冲区大小
	maxNamedCacheSize = 16
)

// state 记录着 decoder 的处理状态
type state uint8

const (
	// stateDecodeHeader 初始值 处于 header 解析状态
	stateDecodeHeader state = iota

	// stateDecodePayload 处于 payload 解析状态
	stateDecodePayload
)

type decoder struct {
	t0         time.Time
	st         socket.TupleRaw
	serverPort socket.Port
	role       role.Role

	payloadLen      uint32
	payloadConsumed uint32
	state           state
	drainBytes      int

	nsc     *namedStatementCache
	reqTime time.Time

	statementName *bufbytes.Bytes
	statement     *bufbytes.Bytes
	describe      *bufbytes.Bytes
	readall       bool
	count         int

	flag    uint8
	packet  any
	tail    []byte // 尾部数据拼接 仅允许拼接一次 避免上一轮切割了部分数据
	partial uint8
}

func NewDecoder(st socket.Tuple, serverPort socket.Port) protocol.Decoder {
	return &decoder{
		st:            st.ToRaw(),
		serverPort:    serverPort,
		statement:     bufbytes.New(maxStatementSize),
		statementName: bufbytes.New(maxStatementNameSize),
		describe:      bufbytes.New(maxDescribeSize),
		nsc:           newNamedStatementCache(maxNamedCacheSize),
	}
}

// Decode 从 zerocopy.Reader 中不断解析来自 Request / Response 的数据 并判断是否能构建成 RoundTrip
//
// # Decode 要求具备容错和自恢复能力 即当出现错误的时候能够适当重置
//
// # PostgreSQL 有两种主要的查询结构 SimpleQuery 和 ExtendQuery
// 前者直接发送 Q 查询即可 后者需要先对语句进行解析和绑定才只能执行查询操作
//
// - SimpleQuery:
// C: Q
// S: R / D / D... / C
//
// - ExtendQuery:
// C: P / B / E / C
// S: R / D / D... / C
//
// # 下面是一个 ExtendQuery 的具体示例
//
// +--------------------+                      +-----------------+
// |     Client         |                      |      Server     |
// +--------------------+                      +-----------------+
// | Parse              |  ---------------->   |                 |
// | stmt_name="s1"     |                      |                 |
// | query="SELECT *    |                      |                 |
// | FROM users WHERE   |                      |                 |
// | id = $1"           |                      |                 |
// +--------------------+                      +-----------------+
// | Bind               |  ---------------->   | ParseComplete   |
// | portal="p1"        |                      |                 |
// | stmt_name="s1"     |                      |                 |
// | param_format=1     |                      |                 |
// | param_value=100    |                      |                 |
// +--------------------+                      +-----------------+
// | Execute            |  ---------------->   | BindComplete    |
// | portal="p1"        |                      |                 |
// | max_rows=0         |                      |                 |
// +--------------------+                      +-----------------+
// | Sync               |  ---------------->   | RowDescription  |
// |                    |                      | (id,name,age)   |
// +--------------------+                      | DataRow         |
// |                    |  <----------------   | (100,"John",30) |
// |                    |                      | CommandComplete |
// |                    |                      | "SELECT 1"      |
// |                    |                      | ReadyForQuery   |
// +--------------------+                      +-----------------+
//
// 从 PostgreSQL Connection 的视角来看 会区分为两个 Stream 以及初始化两个不同的 decoder
// 分别负责 PostgreSQL/request 和 PostgreSQL/response 的解析 如果解析成功后会向上层提交一个 *roundtrip.Object
// 上层拿到 *roundtrip.Object 会进行请求的配对 成功则代表成功捕获到一次 PostgreSQL 请求
//
// 为了尽量模拟近似的 `请求时间`
// Request.Time 从发送的第一个数据包开始计时
// Response.Time 从接收的最后一个数据包停止计时
func (d *decoder) Decode(r zerocopy.Reader, t time.Time) ([]*role.Object, error) {
	d.t0 = t
	d.count++

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

// Free 释放持有的资源
func (d *decoder) Free() {
	d.tail = nil
}

// archive 归档请求
func (d *decoder) archive() []*role.Object {
	if d.isClient() {
		obj := role.NewRequestObject(&Request{
			Size:   d.drainBytes,
			Proto:  PROTO,
			Time:   d.reqTime,
			Host:   d.st.SrcIP,
			Port:   d.st.SrcPort,
			Packet: d.packet,
		})
		d.reset()
		return []*role.Object{obj}
	}

	obj := role.NewResponseObject(&Response{
		Size:   d.drainBytes,
		Time:   d.t0,
		Proto:  PROTO,
		Host:   d.st.SrcIP,
		Port:   d.st.SrcPort,
		Packet: d.packet,
	})
	d.reset()
	return []*role.Object{obj}
}

func (d *decoder) decode(b []byte) ([]byte, bool, error) {
	if d.state == stateDecodeHeader {
		if len(b) < headerLength {
			d.partial++
			d.tail = bytes.Clone(b)
			return nil, false, errDecodeHeader
		}

		// 如果是 StartupMessage 则表示是客户端发起的连接
		// 需要先判断第一个包是否为初始化包 避免 header 解析失败
		//┌─────────────┬─────────────┬───────────────────────────┐
		//│ Length      │ Version     │ KeyVal Pairs              │
		//│ (4B)        │ (4B)        │ (user/database)           │
		//├─────────────┼─────────────┼───────────────────────────┤
		//│ 00 00 00 7C | 00 03 00 00 | user\x00myuser\x00...\x00 |
		//└─────────────┴─────────────┴───────────────────────────┘
		if len(b) >= 8 && d.count == 1 {
			if binary.BigEndian.Uint32(b[4:8]) == startupMessage {
				return nil, false, nil
			}
		}

		if err := d.decodeHeader(b[1:headerLength]); err != nil {
			return nil, false, err
		}

		d.flag = b[0]
		if d.isClient() && d.drainBytes == 0 {
			d.reqTime = d.t0
		}

		b = b[headerLength:]
		d.drainBytes += headerLength
		d.state = stateDecodePayload
	}

	b, complete, err := d.decodePayload(b)
	if err != nil {
		return nil, false, err
	}
	return b, complete, err
}

// decodePayload 解析 payload
//
// 传入 f 用于解析觉得类型的 CommandPacket 返回是否已构建成一个 Request / Response
// 中间状态会记录在 decoder 的各种字段中
func (d *decoder) decodePayload(b []byte) ([]byte, bool, error) {
	if len(b) == 0 {
		return nil, false, nil
	}

	n := d.payloadConsumed + uint32(len(b))
	// 刚好能够拼接成一个数据包
	if n == d.payloadLen {
		d.payloadConsumed += uint32(len(b))
		d.drainBytes += len(b)
		d.state = stateDecodeHeader
		d.readall = true
		return nil, d.decodePacket(b), nil
	}

	// 剩余的数据已经超过一个完整的 payload 所需要的字节
	// 则需要消费剩下的内容并将 tail 返回
	if n > d.payloadLen {
		consumed := d.payloadLen - d.payloadConsumed
		if d.payloadLen < d.payloadConsumed {
			return nil, false, errInvalidBytes
		}
		d.payloadConsumed += consumed
		d.drainBytes += int(consumed)
		d.state = stateDecodeHeader
		d.readall = true
		return b[consumed:], d.decodePacket(b[:consumed]), nil
	}

	d.payloadConsumed += uint32(len(b))
	d.drainBytes += len(b)
	return nil, d.decodePacket(b), nil
}

// reset 重置状态
func (d *decoder) reset() {
	d.state = stateDecodeHeader
	d.payloadConsumed = 0
	d.payloadLen = 0
	d.drainBytes = 0
	d.statementName.Reset()
	d.statement.Reset()
	d.describe.Reset()
	d.flag = 0
	d.readall = false
	d.packet = nil
}

// decodePacket 根据 header 解析的 Flag 选择对应的解析函数
// TODO(mando): 这里仅解析了部分类型 后续待补充
func (d *decoder) decodePacket(b []byte) bool {
	switch d.flag {
	case flagQuery:
		d.decodeQueryPacket(b)

	case flagParse:
		d.decodeParsePacket(b)

	case flagBind:
		d.decodeBindPacket(b)

	case flagCloseOrCommandComplete:
		if !d.isClient() {
			d.decodeCommandCompletePacket(b)
		}

	case flagExecuteOrErrorResponse:
		if !d.isClient() {
			d.decodeErrorPacket(b)
		}

	case flagDescribeOrDataRow:
		if d.isClient() {
			d.decodeDescribePacket(b)
		}

	case flagCloseCompleteOrDescribeResponse:
		if !d.isClient() {
			d.decodeOnlyFlagPacket()
		}
	}

	// 当且仅当数据包被完整被消费且已经构建成 packet 再返回
	if d.readall && d.packet != nil {
		return true
	}
	return false
}

func (d *decoder) isClient() bool {
	return uint16(d.serverPort) == d.st.DstPort
}

// decodeHeader 解析数据包 Header 布局如下

// ┌─────────────────────────────────────────────┐
// │              PostgreSQL Message             │
// ├──────┬─────────┬────────────────────────────┤
// │ Type │ Length  │           Payload          │
// │ (1B) │ (4B)    │          (N Bytes)         │
// └──────┴─────────┴────────────────────────────┘
//
// - Type: 数据包类型 参见 clientFlagNames 和 serverFlagNames
// - Length: 数据包长度 包含 Header 部分
// - Payload: 数据包内容
//
// 这里仅记录 payloadLen 并标记 payloadConsumed
func (d *decoder) decodeHeader(b []byte) error {
	n := binary.BigEndian.Uint32(b)
	d.payloadLen = n
	d.payloadConsumed = headerLength - 1
	return nil
}

type FlagPacket struct {
	Flag string
}

func (p FlagPacket) Name() string {
	return p.Flag
}

func (d *decoder) decodeOnlyFlagPacket() {
	name := clientFlagNames[d.flag]
	if !d.isClient() {
		name = serverFlagNames[d.flag]
	}

	if !d.readall {
		return
	}
	d.packet = &FlagPacket{
		Flag: name,
	}
}

// decodeParsePacket 解析 ParseCommand 数据包 布局如下
//
// ┌─────────┬───────────┬──────────────┬──────────────────────┬─────────────┬───────────────┐
// │  Type   │ Length    │ Statement    │   Query String       │ Num Params  │ Param OIDs    │
// │ (1B)    │ (4B)      │ (str + \0)   │   (str + \0)         │ (2B)        │ [n] (4B each) │
// ├─────────┼───────────┼──────────────┼──────────────────────┼─────────────┼───────────────┤
// │  'P'    │  N + 4    │ "prepared1"  │   SELECT $1::TEXT;   │ 0x0001      │ 0x00000019    │
// │ (0x50)  │ (Big-End) │ + \x00       │   + \x00             │ (n=1 param) │ (OID=25)      │
// └─────────┴───────────┴──────────────┴──────────────────────┴─────────────┴───────────────┘
//
// - Type (1B)
// 固定为 ASCII 字符 'P'（十六进制 0x50）表示预处理语句请求
//
// - Length (4B)
// 大端序整数，包含整个消息长度（含 Type 和 Length 自身）
//
// - Statement (变长)
// 预处理语句名称（客户端定义）以 \x00 结尾的字符串 空字符串表示未命名的临时预处理语句
//
// - Query String (变长)
// 要预处理的 SQL 语句（含参数占位符如 $1、$2）以 \x00 结尾的字符串
//
// - Num Params (2B)
// 大端序整数 表示后续参数类型 OID 的数量 n 若为 0 表示未指定参数类型
//
// - Param OIDs (变长)
// 每个参数的 PostgreSQL 数据类型 OID（四字节大端序整数）
//
// 仅记录 statementName 和 statement 用于 bond 查询做查询映射
func (d *decoder) decodeParsePacket(b []byte) {
	idx := bytes.IndexByte(b, cStringEnd)
	if idx == -1 {
		d.statementName.Write(b)
		return
	}
	d.statementName.Write(b[:idx])

	idx++
	if len(b) <= idx {
		return
	}

	buf := b[idx:]
	idx = bytes.IndexByte(buf, cStringEnd)
	if idx == -1 {
		d.statement.Write(buf)
	} else {
		d.statement.Write(buf[:idx])
	}

	if !d.readall {
		return
	}

	// 每次均需要重置
	name := d.statementName.TrimCStringText()
	d.statementName.Reset()

	statement := d.statement.TrimCStringText()
	d.statement.Reset()

	// 仅记录有效的 name / statement
	if name != "" && statement != "" {
		d.nsc.Set(name, statement)
	}
}

type DescribePacket struct {
	Type   string
	Object string
}

func (p DescribePacket) Name() string {
	return "DescribePacket"
}

// decodeDescribePacket 解析 DescribeCommand 数据包 布局如下
//
// ┌─────────┬──────────┬─────────────┬──────────────┐
// │  Type   │ Length   │  Target     │  Name        │
// │ (1B)    │ (4B)     │  (1B)       │ (str + \0)   │
// ├─────────┼──────────┼─────────────┼──────────────┤
// │  'D'    │  N +4    │  'S'        │ "stmt1"+\x00 │
// │ (0x44)  │ (Big-End)│  or 'P'     │  or portal   │
// └─────────┴──────────┴─────────────┴──────────────┘
//
// Type (1B)
// - 固定为 ASCII 字符 'D'（十六进制 0x44）表示元数据查询请求
//
// - Length (4B)
// 大端序整数 表示消息总长度（包含 Type 和 Length 自身）
//
// _ Target (1B)
// 目标类型标识符:
// 'S'（0x53）: 查询预处理语句的元数据（参数类型和结果列描述）
// 'P'（0x50）: 查询门户的元数据（结果列描述）
//
// - Name (变长)
// 目标名称:
// * Target 为 'S': 预处理语句名称（由 Parse 命令定义）
// * Target 为 'P': 门户名称（由 Bind 命令定义）
func (d *decoder) decodeDescribePacket(b []byte) {
	if len(b) <= 1 {
		return
	}

	idx := bytes.IndexByte(b, cStringEnd)
	if idx == -1 {
		d.describe.Write(b[1:])
	} else {
		if idx == 0 {
			d.describe.Write(b[:1])
		} else {
			d.describe.Write(b[1:idx])
		}
	}

	if !d.readall {
		return
	}
	d.packet = &DescribePacket{
		Type:   string(b[0]),
		Object: d.describe.TrimCStringText(),
	}
}

type QueryPacket struct {
	Statement string
}

func (p QueryPacket) Name() string {
	return "Query"
}

// decodeQueryPacket 解析 QueryCommand 数据包 布局如下
//
// ┌───────────┬─────────────┬──────────────────────────────┬───────┐
// │   Type    │  Length     │       Query String           │  NULL │
// │  (1 Byte) │  (4 Bytes)  │      (Variable Length)       │ (1B)  │
// ├───────────┼─────────────┼──────────────────────────────┼───────┤
// │   'Q'     │  N + 4 +1   │  SELECT * FROM table;        │  \x00 │
// │  (0x51)   │ (Big-Endian)│  ...or other SQL statement...│       │
// └───────────┴─────────────┴──────────────────────────────┴───────┘
//
// - Type (1 Byte)
// 固定为 ASCII 字符 'Q'（十六进制 0x51）表示这是一个查询请求
//
// _ Length (4 Bytes)
// 大端序（Big-Endian）整数 表示整个消息的总字节数（包括 Type 和 Length 自身）
//
// - Query String (变长)
// 要执行的 SQL 语句 以可读的 UTF-8 字符串形式存在（例如 SELECT * FROM users;）
//
// - NULL Terminator (1 Byte)
// 固定为 \x00（零字节）标志查询字符串的结束
//
// 记录最终 statement 时需要注意去除结尾的 NULL 字节
func (d *decoder) decodeQueryPacket(b []byte) {
	d.statement.Write(b)
	if !d.readall {
		return
	}
	d.packet = &QueryPacket{
		Statement: d.statement.TrimCStringText(),
	}
}

// decodeBindPacket 解析 BindCommand 数据包 布局如下
//
// ┌─────────┬──────────┬────────────┬─────────────┬───────────────┬──────────────┐
// │  Type   │ Length   │ Portal     │ Statement   │ Param Formats │ Param Values │
// │ (1B)    │ (4B)     │ (str + \0) │ (str + \0)  │ [n] (2B each) │ [n] (var)    │
// ├─────────┼──────────┼────────────┼─────────────┼───────────────┼──────────────┤
// │  'B'    │  N + 4   │ "my_portal"│ "prepared1" │ 0x0001        │ 0x0004 '123' │
// │ (0x42)  │ (Big-End)│ + \x00     │ + \x00      │ (n=1 format)  │ (1 param)    │
// └─────────┴──────────┴────────────┴─────────────┴───────────────┴──────────────┘
//
// - Type (1B)
// 固定为 'B'（ASCII 十六进制 0x42）表示绑定操作
//
// - Length (4B)
// 大端序整数 包含整个消息长度（含 Type 和 Length 自身）
//
// - Portal (变长)
// 目标门户名称（客户端定义的执行入口）以 \x00 结尾的字符串 为空字符串表示匿名门户
//
// - Statement (变长)
// 预处理语句名称（由 Parse 命令创建）以 \x00 结尾的字符串 为空字符串表示未命名的预处理语句
//
// - Param Formats (变长): 参数格式列表（这里不做解析）
// - Param Values (变长): 参数格式列表（这里不做解析）
//
// 记录最终 statement 时需要注意去除结尾的 NULL 字节
func (d *decoder) decodeBindPacket(b []byte) {
	idx := bytes.IndexByte(b, cStringEnd)
	if idx == -1 {
		return
	}

	idx++
	if len(b) <= idx {
		return
	}

	buf := b[idx:]
	idx = bytes.IndexByte(buf, cStringEnd)
	if idx == -1 {
		d.statementName.Write(buf)
	} else {
		d.statementName.Write(buf[:idx])
	}

	if !d.readall {
		return
	}
	d.packet = &QueryPacket{
		Statement: d.nsc.Get(d.statementName.TrimCStringText()),
	}
}

type CommandCompletePacket struct {
	Command string
	Rows    int
}

func (p CommandCompletePacket) Name() string {
	return "CommandComplete"
}

// decodeCommandCompletePacket 解析 CommandCompleteCommand 数据包 布局如下
//
// ┌───────────┬─────────────┬─────────────────────────────┬───────┐
// │   Type    │  Length     │       Command Tag           │  NULL │
// │  (1 Byte) │  (4 Bytes)  │      (Variable Length)      │ (1B)  │
// ├───────────┼─────────────┼─────────────────────────────┼───────┤
// │   'C'     │  N + 4      │       INSERT 0 1            │  \x00 │
// │  (0x43)   │ (Big-Endian)│    ...Command or Tag...     │       │
// └───────────┴─────────────┴─────────────────────────────┴───────┘
//
// - Type (1B)
// 固定为 ASCII 字符 'C'（十六进制 0x43）表示命令执行完成
//
// - Length (4B)
// 大端序整数 包含整个消息的长度（包括 Type 和 Length 自身）
//
// - Command Tag (变长)
// 命令执行结果标签 始终以 \x00 终止 格式取决于操作类型:
// * SELECT：SELECT <Rows>（如 SELECT 3）
// * INSERT/UPDATE/DELETE: <Op> <OID> <Rows>（如 INSERT 0 1）
// * DDL: 直接返回操作名（如 CREATE TABLE）
//
// OID 非重要字段 不做记录
func (d *decoder) decodeCommandCompletePacket(b []byte) {
	cmdPacket := &CommandCompletePacket{}

	buf := b
	if bytes.HasSuffix(b, []byte{cStringEnd}) {
		buf = buf[:len(buf)-1]
	}

	fields := strings.Fields(string(buf))
	switch len(fields) {
	case 3:
		cmdPacket.Command = fields[0]
		cmdPacket.Rows, _ = strconv.Atoi(fields[2])
	case 2:
		cmdPacket.Command = fields[0]
		cmdPacket.Rows, _ = strconv.Atoi(fields[1])
	}

	if !d.readall {
		return
	}
	d.packet = cmdPacket
}

type ErrorPacket struct {
	Severity     string
	SQLStateCode string
	Message      string
}

func (p ErrorPacket) Name() string {
	return "Error"
}

// decodeErrorPacket 解析 ErrorCommand 数据包 布局如下
//
// ┌───────────┬─────────────┬──────────────────────────────────────────┐
// │   Type    │  Length     │          Error Fields (Variable)         │
// │  (1 Byte) │  (4 Bytes)  │  ┌──────┬──────────────┬──────┬─────────┐│
// ├───────────┼─────────────┤  │ Code │  Value       │ ...  │ NULL    ││
// │   'E'     │  N + 4      │  │ (1B) │ (str + \0)   │      │ (1B)    ││
// │  (0x45)   │ (Big-Endian)├──┼──────┼──────────────┼──────┼─────────┘│
// └───────────┴─────────────┴──┴──────┴──────────────┴──────┴──────────┘
//
// - Type (1B)
// 固定为 ASCII 字符 'E'（十六进制 0x45）表示错误响应
//
// - Length (4B)
// 大端序整数 包含整个消息的长度（包括 Type 和 Length 自身）
//
// - Error Fields (变长)
// 由多个 键值对 组成 每个字段结构为:
//
// ┌──────┬─────────────────────┬──────┐
// │ Code │   Value (字符串)     │ NULL │
// │ (1B) │   (以 \x00 结尾)     │ (1B) │
// └──────┴─────────────────────┴──────┘
// 常见字段代码（Code）:
// * S (0x53): 错误级别（如 FATAL、ERROR、WARNING）
// * M (0x4D)
// * C (0x43): SQLSTATE 错误码
//
// 其余扩展字段非重点内容 不做记录
func (d *decoder) decodeErrorPacket(b []byte) {
	errPacket := &ErrorPacket{}
	var cursor int
	for len(b) > cursor {
		key := b[cursor]
		cursor++

		idx := bytes.IndexByte(b[cursor:], cStringEnd)
		if idx < 0 {
			break
		}

		val := b[cursor : cursor+idx] // 按需拷贝内存
		switch key {
		case 'S':
			errPacket.Severity = string(val)
		case 'C':
			errPacket.SQLStateCode = string(val)
		case 'M':
			errPacket.Message = string(val)
		default:
			continue
		}

		cursor += idx + 1
	}

	if !d.readall {
		return
	}
	d.packet = errPacket
}
