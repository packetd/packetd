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

package predis

import (
	"bytes"
	"io"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/splitio"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

const (
	PROTO = "Redis"
)

func newError(format string, args ...any) error {
	format = "redis/decoder: " + format
	return errors.Errorf(format, args...)
}

var (
	errInvalidBytes     = newError("invalid bytes")
	errDecodeBulkString = newError("decode BulkString failed")
	errDecodeN          = newError("decode NField failed")
)

// decoder Redis RESP 协议解析器
//
// decoder 负责解析流式的 RESP 数据 使用了 register 作为寄存器缓存解析状态并将其记录在 stack 中
// 在解析的同时会记录 Request / Response 的字节长度作为 Content-Length
type decoder struct {
	t0         time.Time // 记录最后一次 decode 的时间
	st         socket.TupleRaw
	serverPort socket.Port

	role    role.Role
	reqTime time.Time // 请求接收到 command 的时间

	drainBytes  int      // 已经读取的字节数
	dataType    DataType // 记录本轮解析的数据类型
	command     string   // 记录 RESP 命令
	prevSeenCmd bool     // 记录前一次解析是否已经遇到过命令
	stack       *stack
}

func NewDecoder(st socket.Tuple, serverPort socket.Port) protocol.Decoder {
	return &decoder{
		st:         st.ToRaw(),
		serverPort: serverPort,
		stack:      newStack(),
	}
}

// reset 重置单次请求状态
func (d *decoder) reset() {
	d.drainBytes = 0
	d.stack.reset()
	d.role = ""
	d.dataType = ""
	d.prevSeenCmd = false
}

// Decode 从 zerocopy.Reader 中不断解析来自 Request / Response 的数据 并判断是否能构建成 RoundTrip
//
// # Decode 要求具备容错和自恢复能力 即当出现错误的时候能够适当重置
//
// 解析使用 RESP 2.0 标准协议 RESP 是一个支持多种数据类型的序列化协议 在 RESP 中数据的类型依赖于首字节
//
// - 单行字符串 (SimpleStrings): 响应的首字节是 "+"
// - 错误 (Errors): 响应的首字节是 "-"
// - 整型 (Integers): 响应的首字节是 ":"
// - 多行字符串 (BulkStrings): 响应的首字节是"\$"
// - 数组 (Arrays): 响应的首字节是 "*"
//
// # RESP 在 Redis 中作为一个请求-响应协议以如下方式使用
//
// - 客户端以 BulkStrings 类型数组的方式发送命令给服务器端
// - 服务器端根据命令的具体实现返回某一种 RESP 数据类型
//
// +-----------------+                      +-----------------+
// |     Client      |                      |      Server     |
// +-----------------+                      +-----------------+
// | *2\r\n          |  ----------------->  |                 |
// | $3\r\n          |                      |                 |
// | GET\r\n         |                      |                 |
// | $4\r\n          |                      |                 |
// | key1\r\n        |                      |                 |
// |                 |  <-----------------  | $6\r\n          |
// |                 |                      | value1\r\n      |
// +-----------------+                      +-----------------+
//
// # Request
// *2\r\n: 表示一个包含 2 个元素的数组。
// $3\r\nGET\r\n: 第一个元素是长度为 3 的字符串 GET
// $4\r\key1\r\n: 第二个元素是长度为 4 的字符串 key1
//
// # Response
// $6\r\nvalue1\r\n: 表示一个长度为 6 的批量字符串 value1
//
// 从 Redis Connection 的视角来看 会区分为两个 Stream 以及初始化两个不同的 decoder
// 分别负责 Redis/request 和 Redis/response 的解析 如果解析成功后会向上层提交一个 *roundtrip.Object
// 上层拿到 *roundtrip.Object 会进行请求的配对 成功则代表成功捕获到一次 Redis 请求
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

	var objs []*role.Object
	lr := splitio.NewReader(b)

	// 需要持续推进解析 直到本次 Read 内容已经全部读取完毕
	// 确保能够拼接成完整的 RESP 数据包
	for !lr.EOF() {
		obj, err := d.decode(lr)
		if err != nil {
			if errors.Is(err, io.ErrShortBuffer) {
				continue
			}
			d.reset() // 出现任何错误都应该重置链接 从头开始探测
			return nil, err
		}

		if obj == nil {
			continue
		}

		objs = append(objs, obj)
		d.reset()
		return objs, err
	}
	return objs, nil
}

// Free 释放持有的资源
func (d *decoder) Free() {}

// archive 归档请求
func (d *decoder) archive() *role.Object {
	if d.role == role.Request {
		obj := role.NewRequestObject(&Request{
			Command: d.command,
			Size:    d.drainBytes,
			Proto:   PROTO,
			Time:    d.reqTime,
			Host:    d.st.SrcIP,
			Port:    d.st.SrcPort,
		})
		return obj
	}

	obj := role.NewResponseObject(&Response{
		DataType: string(d.dataType),
		Size:     d.drainBytes,
		Time:     d.t0,
		Proto:    PROTO,
		Host:     d.st.SrcIP,
		Port:     d.st.SrcPort,
	})
	return obj
}

// decodeContinue 继续解析后续数据包
//
// 当且仅当 stack 非空时会进入此逻辑
func (d *decoder) decodeContinue(reg *register, r *splitio.Reader) (bool, error) {
	var complete bool
	var err error

	d.stack.push(reg)
	switch reg.dataType {
	case Array:
		complete, err = d.decodeArray(r)
		if err != nil {
			return false, err
		}
		d.dataType = Array

	case BulkStrings:
		complete, err = d.decodeBulkStrings(r)
		if err != nil {
			return false, err
		}
		d.dataType = BulkStrings
	}
	return complete, err
}

// decode 为真正的数据解析入口
//
// 支持递归解析 优先解析 Array 其次是 BulkStrings 再者才是其他
func (d *decoder) decode(r *splitio.Reader) (*role.Object, error) {
	// 进入此逻辑表示 在单个 TCP 包中未能完整地解析出 Request / Response 的所有内容
	// 因此需要根据 stack 中的状态继续进入下一轮的解析
	reg := d.stack.pop()
	if reg != nil {
		complete, err := d.decodeContinue(reg, r)
		if err != nil {
			return nil, err
		}
		if !complete || !d.stack.empty() {
			return nil, nil
		}
		return d.archive(), nil
	}

	line, eof := r.ReadLine()
	if eof {
		return nil, io.ErrShortBuffer
	}

	complete := true
	switch line[0] {
	case '*':
		// 解码 Array
		//
		// 客户端使用 RESP 数组发送命令到 Redis 服务端 RESP 数组使用如下格式发送
		// 以 [*] 为首字符 接着是表示数组中元素个数的十进制数 最后以 CRLF 结尾
		// 数组的元素可以是任意的 RESP 数据类型 包括数据本身 即嵌套数组
		//
		// "*5\r\n"
		// ":1\r\n"
		// ":2\r\n"
		// ":3\r\n"
		// ":4\r\n"
		// "$6\r\n"
		// "foobar\r\n"
		n, err := decodeNTrim(line[1:])
		if err != nil {
			return nil, errDecodeN
		}
		d.stack.push(&register{
			dataType: Array,
			arrayN:   n,
		})
		complete, err = d.decodeArray(r)
		if err != nil {
			return nil, err
		}
		d.dataType = Array

	case '$':
		// 解码 BulkStrings
		//
		// 多行字符串被用来表示最大 512MB 的二进制安全字符串 编码方式为
		// - [\$] 后面跟着组成字符串的字节数(前缀长度) + CRLF
		// - 实际的字符串数据 +  + CRLF
		//
		// "$6\r\nfoobar\r\n"
		n, err := decodeNTrim(line[1:])
		if err != nil {
			return nil, errDecodeN
		}
		d.stack.push(&register{
			dataType:     BulkStrings,
			bulkStringsN: n,
		})
		complete, err = d.decodeBulkStrings(r)
		if err != nil {
			return nil, err
		}
		d.dataType = BulkStrings

	case '-':
		// 解码 Errors
		//
		// [-] 后面跟着一个字符串 跟 SimpleStrings 的区别是错误被客户端当作异常处理
		// 组成错误类型的字符串是错误消息自身
		//
		// "-Error message\r\n"
		d.decodeOneLine(line[1:])
		d.dataType = Errors

	case '+':
		// 解码 SimpleStrings
		//
		// [+] 后面跟着一个不包含回车或换行字符的字符串 (不允许出现换行) 以 CRLF 结尾
		// 如 redis 命令在成功时回复 "OK" 即 SimpleStrings 用以下 5 个字节编码
		//
		// "+OK\r\n"
		d.decodeOneLine(line[1:])
		d.dataType = SimpleStrings

	case ':':
		// 解码 Integers
		//
		// [:] 后面跟着一个整型数值 整数有效值需要在有符号64位整数范围内
		//
		// ":1000\r\n"
		d.decodeOneLine(line[1:])
		d.dataType = Integers

	default:
		if d.stack.empty() {
			return nil, errInvalidBytes
		}
		d.decodeOneLine(line)
	}

	// 如果已经标识为完成态 则需要归档请求
	if complete {
		if !d.stack.empty() {
			return nil, nil
		}
		return d.archive(), nil
	}
	return nil, nil
}

func (d *decoder) isClient() bool {
	return uint16(d.serverPort) == d.st.DstPort
}

// decodeBulkString 解析 BulkStrings
//
// 先从 stack pop 出最近的一次状态 然后持续 ReadLine decode
func (d *decoder) decodeBulkStrings(r *splitio.Reader) (bool, error) {
	reg := d.stack.pop()
	if reg == nil {
		return false, nil
	}

	if reg.bulkStringsN <= 0 {
		return true, nil
	}

	for {
		line, eof := r.ReadLine()
		if eof {
			d.stack.push(reg) // 请求还没结束 那就进入下一轮
			return false, io.ErrShortBuffer
		}

		n := len(line)
		if bytes.HasSuffix(line, splitio.CharCRLF) {
			n = len(line) - 2
		}

		// 判断是否为 Request 并记录命令
		if d.role != role.Request && d.isClient() {
			if cmd := normalizeCommand(line); cmd != "" {
				d.role = role.Request
				d.command = cmd
				d.prevSeenCmd = true
				d.reqTime = d.t0
			}
		} else {
			// 确定是否有子命令
			if d.prevSeenCmd {
				if cmd := normalizeSubCommand(d.command, line); cmd != "" {
					d.command = d.command + " " + cmd
				}
				d.prevSeenCmd = false
			}
		}

		reg.bulkStringsConsume += n
		d.drainBytes += n

		if reg.bulkStringsConsume == reg.bulkStringsN {
			return true, nil
		}
		if reg.bulkStringsConsume > reg.bulkStringsN {
			return false, errDecodeBulkString
		}
	}
}

// decodeOneLine 解析 +/-/: 三个标识符的数据
func (d *decoder) decodeOneLine(line []byte) {
	if bytes.HasSuffix(line, splitio.CharCRLF) {
		d.drainBytes += len(line) - 2
	} else {
		d.drainBytes += len(line)
	}
}

// decodeArray 递归解析 Array 数据
//
// 解析失败时需要更新寄存器状态并重新入栈
func (d *decoder) decodeArray(r *splitio.Reader) (bool, error) {
	reg := d.stack.pop()
	if reg == nil {
		return false, nil
	}

	for i := 0; i < reg.arrayN; i++ {
		if _, err := d.decode(r); err != nil {
			reg.arrayN -= reg.arrayConsume
			reg.arrayConsume = 0
			d.stack.push(reg)
			return false, err
		}
		reg.arrayConsume++
	}
	return true, nil
}

// decodeNTrim 解析 N 即 BulkStrings 或者 Array 该有的长度
func decodeNTrim(b []byte) (int, error) {
	if bytes.HasSuffix(b, splitio.CharCRLF) {
		return strconv.Atoi(string(b[:len(b)-2]))
	}
	return strconv.Atoi(string(b))
}

// register 中间状态寄存器
//
// 在 decoder 的解析过程中 由于 TCP 层的数据已经被切割 即不保证单个 TCP 包可以拿到完整的请求数据
// 另外 RESP 还支持嵌套数组 所以参考了编程语言的【函数栈】设计 该方式可以让解析时可以随意【挂起】
// 考虑一种情况 假设本次 Redis 请求返回了一个 Array 并且每个数据的每个元素都是 Array
//
// Response Array
// - item1: Array
//   - BulkStrings
//   - BulkStrings
//   - ...
//
// - item2: Array
//   - BulkStrings
//   - BulkStrings
//   - ...
//
// 则在解析的过程中 解析函数可能在任意的环节中断（只来了部分数据）然后再下一个 TCP 包再继续
// 这是非常有可能的 因为 IP 包的最大长度为 65535 而 redis 单 value 的上限远不止于此
// 终上所述 需要在每一轮解析之前先入栈 然后在解析时再出栈 同时解析要支持递归
type register struct {
	dataType           DataType // RESP 数据类型
	bulkStringsN       int      // 期待的 BulkStrings 长度
	bulkStringsConsume int      // 实际消费的 BulkStrings 长度
	arrayN             int      // 期待的 Array 长度
	arrayConsume       int      // 实际消费的 Array 长度
}

// stack 存放 register 的栈实现
type stack struct {
	array []*register
}

func newStack() *stack {
	return &stack{array: make([]*register, 0)}
}

func (s *stack) push(reg *register) {
	s.array = append(s.array, reg)
}

func (s *stack) pop() *register {
	if s.empty() {
		return nil
	}
	reg := s.array[len(s.array)-1]
	s.array = s.array[0 : len(s.array)-1]
	return reg
}

func (s *stack) empty() bool {
	return len(s.array) == 0
}

func (s *stack) reset() {
	s.array = s.array[:0]
}
