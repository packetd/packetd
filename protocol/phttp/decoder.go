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

package phttp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/bufpool"
	"github.com/packetd/packetd/internal/splitio"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

func newError(format string, args ...any) error {
	format = "http/decoder: " + format
	return errors.Errorf(format, args...)
}

var (
	charHTTP11     = []byte("HTTP/1.1") // 目前仅支持 HTTP1.1 协议版本
	charHTTP11CRLF = append(charHTTP11, splitio.CharCRLF...)
	charEndOfBody  = append([]byte("0"), splitio.CharCRLF...)
)

// state 记录着 decoder 的处理状态
type state uint8

const (
	// stateDecodeProtocol 初始值
	// 处于此状态时正在匹配协议行 如 HTTP/1.1
	stateDecodeProtocol state = iota

	// stateDecodeHeader 解析 header 状态
	// 处于此状态时已经确定了是 Request 或者是 Response 并开始解析 header
	stateDecodeHeader

	// stateDecodeBody 解析 body 状态
	// 处于此状态时 header 已经处理完毕 开始解析 body 内容
	stateDecodeBody
)

// decoder HTTP1.1 协议解析器
//
// decoder 利用了 http.ReadRequest / http.ReadResponse 方法对协议的 Protocol 以及 Header 进行解析
// 同时 decoder 并不支持存储和流式 decode body 内容 仅计算 Content-Length
//
// 对于一个 Stream 而言 其 role 只能为 Request / Response 二者其一
// 因为在同一个 HTTP 连接中 通信的双方一定是有严格区分 Server/Client 端的
type decoder struct {
	st                socket.TupleRaw
	serverPort        socket.Port
	role              role.Role // 记录当前 decoder 的角色
	t0                time.Time // 记录最后一次 decode 的时间
	rbuf              *bytes.Buffer
	bodyBuf           bytes.Buffer // body 内容存储
	reqTime           time.Time    // 请求接收到的时间
	chunked           bool         // 记录当次请求是否为 chunked 模式
	drainBytes        int          // 已经读取的 body 字节数
	expectedBytes     int          // 期待读取的 body 字节 在 chunked 模式下位 0
	enableBodyCapture bool         // 是否启用 body 捕获
	maxBodySize       int          // 最大 body 捕获大小
	captureBody       bool         // 是否捕获 body 内容, 默认不捕获

	state        state
	obj          *role.Object
	headBodyLine []byte
}

const defaultMaxBodySize = 102400 // 100KB

func NewDecoder(st socket.Tuple, serverPort socket.Port, options common.Options) protocol.Decoder {

	// 只有开启了 body 捕获才会捕获 body
	enableBodyCapture, err := options.GetBool("enableBody")

	// 获取最大 body 捕获大小, 默认为 100KB
	maxBodySize, err := options.GetInt("maxBodySize")
	if err != nil || maxBodySize <= 0 {
		maxBodySize = defaultMaxBodySize
	}

	return &decoder{
		st:                st.ToRaw(),
		serverPort:        serverPort,
		rbuf:              bufpool.Acquire(),
		enableBodyCapture: enableBodyCapture,
		maxBodySize:       maxBodySize,
	}
}

// reset 重置单次请求状态
func (d *decoder) reset() {
	d.state = stateDecodeProtocol
	d.obj = nil
	d.drainBytes = 0
	d.expectedBytes = 0
	d.chunked = false
	d.rbuf.Reset()
	d.captureBody = false
	d.bodyBuf.Reset()
	d.headBodyLine = nil
}

// afterResponseHeader 在解析完 Response Header 之后调用
func (d *decoder) afterResponseHeader(resp *Response) {
	if !d.enableBodyCapture {
		d.captureBody = false
		return
	}
	ct := resp.Header.Get("Content-Type")
	d.captureBody = isJSONContentType(ct)
}

func (d *decoder) appendBodyChunk(p []byte) {
	// 如果未启用 body 捕获 则直接返回
	if !d.enableBodyCapture {
		return
	}
	if !d.captureBody || d.bodyBuf.Len() >= d.maxBodySize {
		return
	}
	remain := d.maxBodySize - d.bodyBuf.Len()
	if len(p) > remain {
		p = p[:remain]
	}
	d.bodyBuf.Write(p)
}

// 归档响应时写入 JSON
func (d *decoder) archiveResponseBody(resp *Response) {
	if !d.enableBodyCapture {
		return
	}
	// 如果不符合捕获条件 则直接返回
	if !d.captureBody {
		return
	}
	b := d.bodyBuf.Bytes()
	// 去除尾部可能的 CRLF 与空白
	b = bytes.TrimSpace(bytes.TrimSuffix(b, []byte("\r\n")))
	if len(b) == 0 {
		return
	}
	if json.Valid(b) {
		resp.Body = json.RawMessage(append([]byte(nil), b...))
	}
}

// archive 归档请求
func (d *decoder) archive() error {
	if d.obj == nil || d.obj.Obj == nil {
		return newError("role (%s) got nil obj", d.role)
	}
	switch obj := d.obj.Obj.(type) {
	case *Request:
		obj.Size = d.decideContentLength()
		obj.Host = d.st.SrcIP
		obj.Port = d.st.SrcPort
		obj.Chunked = d.chunked
		obj.Time = d.reqTime

	case *Response:
		obj.Size = d.decideContentLength()
		obj.Time = d.t0 // response 的时间以接收到的最后一个字节为准
		obj.Host = d.st.SrcIP
		obj.Port = d.st.SrcPort
		obj.Chunked = d.chunked
		d.archiveResponseBody(obj)

	}
	return nil
}

// Free 释放持有的资源
func (d *decoder) Free() {
	d.obj = nil
	bufpool.Release(d.rbuf)
}

// Decode 从 zerocopy.Reader 中不断解析来自 Request / Response 的数据 并判断是否能构建成 RoundTrip
//
// # Decode 要求具备容错和自恢复能力 即当出现错误的时候能够适当重置
//
// 对于一个如下的标准的 HTTP 协议请求
//
// # REQUEST 协议格式
//
// GET /index.html HTTP/1.1
// Host: www.example.com
// User-Agent: Gecko/20100101 Firefox/91.0
// Accept: application/json
// Accept-Encoding: gzip
// Connection: keep-alive
// ...<Body Payload>...
//
// # RESPONSE 协议格式
//
// HTTP/1.1 200 OK
// Date: Wed, 18 Apr 2024 12:00:00 GMT
// Server: Apache/2.4.1 (Unix)
// Last-Modified: Wed, 18 Apr 2024 11:00:00 GMT
// Content-Length: 0
// Content-Type: application/json; charset=UTF-8
// ...<Body Payload>...
//
// 从 HTTP Connection 的视角来看 会区分为两个 Stream 以及初始化两个不同的 decoder
// 分别负责 HTTP/request 和 HTTP/response 的解析 如果解析成功后会向上层提交一个 *roundtrip.Object
// 上层拿到 *roundtrip.Object 会进行请求的配对 成功则代表成功捕获到一次 HTTP 请求
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
	scan := splitio.NewScanner(b) // 按行处理数据
	for scan.Scan() {
		obj, err := d.decode(scan.Bytes())
		if err != nil {
			d.reset() // 出现任何错误都应该重置链接 从头开始探测
			return nil, err
		}
		if obj == nil {
			continue
		}

		objs = append(objs, obj)
		return objs, nil
	}

	return nil, nil
}

func (d *decoder) decode(line []byte) (*role.Object, error) {
	obj, err := d.decodeLine(line)
	if err != nil {
		return nil, err
	}

	// fast-path:
	// 不是所有的所有 HTTP 请求均携带了 body 因此当 decodeLine 结束后需要优先判断下请求是否已经结束
	// 在非 chunked 模式下如果 body 本身无内容 则当 decode 已经结束
	if d.state == stateDecodeBody && !d.chunked && d.expectedBytes == 0 {
		return d.decodeBody(nil)
	}

	return obj, nil
}

// decodeLine 逐行解析所有数据
func (d *decoder) decodeLine(line []byte) (*role.Object, error) {
	// 1) 处理协议行
	if d.state == stateDecodeProtocol {
		ok := d.decodeHeadLine(line)
		if !ok {
			return nil, nil
		}
		d.state = stateDecodeHeader // 已经成功解析到协议行 则下一状态为 decodeHeader
		return nil, nil
	}

	// 2) 处理 header
	if d.state == stateDecodeHeader {
		switch d.role {
		case role.Request:
			return nil, d.decodeRequestHeader(line)

		case role.Response:
			return nil, d.decodeResponseHeader(line)
		}
	}

	// 3) 处理 body
	return d.decodeBody(line)
}

// decodeHeadLine 逐行解析 header 数据
// role 一旦确定 在 decoder 的声明周期内不会发生变化
func (d *decoder) decodeHeadLine(line []byte) bool {
	switch d.role {
	case role.Request:
		return d.decodeRequestHeadLine(line)

	case role.Response:
		return d.decodeResponseHeadLine(line)

	default: // 需要探测此 stream 是 client 或者 server 加速后续判断
		if d.decodeRequestHeadLine(line) {
			return true
		}
		if d.decodeResponseHeadLine(line) {
			return true
		}
		return false
	}
}

// decodeRequestHeadLine 解析请求 Request 协议首行 如 `GET /index.html HTTP/1.1\r\n`
func (d *decoder) decodeRequestHeadLine(line []byte) bool {
	if bytes.HasSuffix(line, charHTTP11CRLF) {
		d.rbuf.Write(line)
		d.role = role.Request
		return true
	}
	return false
}

// decodeResponseHeadLine 解析请求 Response 协议首行 如 `HTTP/1.1 200 OK\r\n`
func (d *decoder) decodeResponseHeadLine(line []byte) bool {
	if bytes.HasPrefix(line, charHTTP11) && bytes.HasSuffix(line, splitio.CharCRLF) {
		d.rbuf.Write(line)
		d.role = role.Response
		return true
	}
	return false
}

// decodeBody 逐行解析 body 内容
func (d *decoder) decodeBody(line []byte) (*role.Object, error) {
	complete, err := d.drainBody(line)
	if err != nil {
		return nil, err
	}
	if !complete {
		return nil, nil
	}

	// complete 代表着 role 所在一方的请求已经结束
	obj := d.obj
	d.reset()
	return obj, nil
}

// decodeRequestHeader 解析 Request Header
//
// Header 一般以 \r\n 作为单行的换行符 并且最后一行的 len 为空
func (d *decoder) decodeRequestHeader(line []byte) error {
	d.rbuf.Write(line)
	if !bytes.Equal(line, splitio.CharCRLF) {
		return nil
	}

	defer d.rbuf.Reset()
	r, err := http.ReadRequest(bufio.NewReaderSize(d.rbuf, d.rbuf.Len()))
	if err != nil {
		return err
	}

	d.state = stateDecodeBody
	d.chunked = checkChunkedEncoding(r.TransferEncoding) && r.ContentLength < 0
	if r.ContentLength > 0 {
		d.expectedBytes = int(r.ContentLength)
	}

	d.reqTime = d.t0
	d.obj = role.NewRequestObject(fromHTTPRequest(r))
	return nil
}

// decodeResponseHeader 解析 Response Header
//
// Header 一般以 \r\n 作为单行的换行符 并且最后一行的 len 为空
func (d *decoder) decodeResponseHeader(line []byte) error {
	d.rbuf.Write(line)
	if !bytes.Equal(line, splitio.CharCRLF) {
		return nil
	}

	defer d.rbuf.Reset()
	r, err := http.ReadResponse(bufio.NewReaderSize(d.rbuf, d.rbuf.Len()), nil)
	if err != nil {
		return err
	}

	d.state = stateDecodeBody
	d.chunked = checkChunkedEncoding(r.TransferEncoding) && r.ContentLength < 0
	if r.ContentLength > 0 {
		d.expectedBytes = int(r.ContentLength)
	}

	d.obj = role.NewResponseObject(fromHTTTResponse(r))

	resp := fromHTTTResponse(r)
	d.afterResponseHeader(resp)
	return nil
}

// drainBody 排空 Request / Response body 内容
//
// 当且仅当 Request / Response 已经被正确解析出来后才会进入 drainBody state
// 对于 HTTP Body 程序不做解析 但要正确读取并处理其字节流 这里需要分两种情况讨论 chunked 是否为 true
//
// (1) Chunked 为 false
// 要求对有`明确的合理的` Content-Length 无论是 Request 或是 Response
// 这样程序可以解析确定字节长度的数据内容
//
// (2) Chunked 为 true
// 如果一个 HTTP 消息的 Transfer-Encoding 消息头的值为 chunked 那其消息体由数量未定的块组成 并以最后一个大小为 0 的块为结束
// 每一个非空的块都以该块包含数据的字节数（字节数以十六进制表示）开始并跟随一个 CRLF 然后是数据本身 最后块 CRLF 结束
//
// https://datatracker.ietf.org/doc/html/rfc9112#name-chunked-transfer-coding
// 根据 rfc 文档中的描述 chunked body 中可能还携带 chunk-ext（需要被删除）
// 而且这里的 `数据包含的字节数使用十六进制表示` 中所指的 16 进制 在解析前并不知道其真实的字节位数 所以还要经过探测
//
// https://httpwg.org/specs/rfc7230.html#rfc.section.3.3.3 文档中也提供了一些细节
// - 比如对于 1xx / 204 / 304 请求是没有 HTTP Response Body 内容的
// - 比如同时指定了 Chunked 但是又设置了 Content-Length 应当成异常链接处理
//
//	chunked-body   = *chunk
//	                 last-chunk
//	                 trailer-section
//	                 CRLF
//
//	chunk          = chunk-size [ chunk-ext ] CRLF
//	                 chunk-data CRLF
//	chunk-size     = 1*HEXDIG
//	last-chunk     = 1*("0") [ chunk-ext ] CRLF
//
//	chunk-data     = 1*OCTET ; a sequence of chunk-size octets
//
// chunked body 示例
// ---------------------------------------
// | 25                                  |
// | This is the data in the first chunk |
// | 1C                                  |
// | and this is the second one          |
// | 3                                   |
// | con                                 |
// | 8                                   |
// | sequence                            |
// | 0                                   |
// ---------------------------------------
//
// 前两个块的数据中包含有显式的 \r\n 字符
//
// "This is the data in the first chunk\r\n"      (37 字符 => 十六进制: 0x25)
// "and this is the second one\r\n"               (28 字符 => 十六进制: 0x1C)
// "con"                                          (3  字符 => 十六进制: 0x03)
// "sequence"                                     (8  字符 => 十六进制: 0x08)
//
// 编码的数据需要以 0 长度的块（ "0\r\n\r\n"）结束 解码后数据
//
// This is the data in the first chunk
// and this is the second one
// consequence
//
// 返回的 bool 为是否结束请求以及解析是否失败
func (d *decoder) drainBody(line []byte) (bool, error) {
	// 记录首行 body 内容 对于 chunked 请求 首行理应记录着真正的 chunked-size
	if d.chunked && len(d.headBodyLine) == 0 && bytes.HasSuffix(line, splitio.CharCRLF) && len(line) > 2 {
		cloned := make([]byte, len(line)-2) // 移除 `\r\n`
		copy(cloned, line)
		d.headBodyLine = cloned
	}

	d.drainBytes += len(line)

	// 非 chunked 模式下需要读取 expectedBytes 字节内容
	if !d.chunked {
		d.appendBodyChunk(line)
		if d.drainBytes == d.expectedBytes {
			if err := d.archive(); err != nil {
				return false, err
			}
			return true, nil
		}
		if d.drainBytes > d.expectedBytes {
			return false, newError("drainBytes %d greater than expectedBytes %d", d.drainBytes, d.expectedBytes)
		}
		return false, nil
	}

	// chunked 模式下非结束符则进行下一轮读取
	//
	// 已经读取到 body 末尾标识
	// 5bytes `\r\n\0\r\n`
	if bytes.Equal(line, charEndOfBody) {
		d.drainBytes -= 5
		if err := d.archive(); err != nil {
			return false, err
		}
		return true, nil
	}

	// 属于正常的 data 数据 block 不做调整（可能会有偏差）
	if len(line) > 8 {
		d.appendBodyChunk(line)
		return false, nil
	}

	// 以 CRLF 结尾的情况 尝试解析出字节 block 大小
	if bytes.HasSuffix(line, splitio.CharCRLF) {
		// 1) 空行已经在 EOB 中处理了
		if len(line) == 2 {
			return false, nil
		}

		// 2) 否则尝试解析出 hexdig
		if _, err := parseHexUint(line[:len(line)-2]); err == nil {
			d.drainBytes -= len(line)
		} else {
			d.drainBytes -= 2 // 减去 `\r\n`
		}
	}
	return false, nil
}

// decideContentLength 判断决定使用以哪个结果作为 Content-Length
//
// 1）对于非 chunked 请求 统一使用 d.drainBytes
// 2）对于 chunked 请求 需要先判断 FBL 是否为有效的 hexdig 同时要求 n > d.drainBytes 才使用 即 d.drainBytes 作为兜底值
func (d *decoder) decideContentLength() int {
	if !d.chunked {
		return d.drainBytes
	}

	n, err := parseHexUint(d.headBodyLine)
	if err != nil {
		return d.drainBytes
	}
	if int(n) > d.drainBytes {
		return int(n)
	}
	return d.drainBytes
}

// parseHexUint 将 16 进制所代表的字节解析成 uint64 数据类型
func parseHexUint(v []byte) (uint64, error) {
	if len(v) == 0 {
		return 0, errors.New("empty hex number for chunk length")
	}

	var n uint64
	for i, b := range v {
		switch {
		case '0' <= b && b <= '9':
			b = b - '0'
		case 'a' <= b && b <= 'f':
			b = b - 'a' + 10
		case 'A' <= b && b <= 'F':
			b = b - 'A' + 10
		default:
			return 0, errors.New("invalid byte in chunk length")
		}
		if i == 16 {
			return 0, errors.New("http chunk length too large")
		}
		n <<= 4
		n |= uint64(b)
	}
	return n, nil
}

// checkChunkedEncoding 检查 HTTP Header 中的 Transfer-Encoding 模式是否为 chunked
func checkChunkedEncoding(te []string) bool {
	return len(te) > 0 && te[0] == "chunked"
}

// isJSONContentType 检查 Content-Type 是否为 JSON 格式
func isJSONContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "application/json") || strings.Contains(ct, "text/json")
}
