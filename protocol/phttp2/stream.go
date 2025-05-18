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
	"time"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/bufpool"
	"github.com/packetd/packetd/protocol/role"
)

const (
	PROTO = "HTTP/2"
)

var (
	errInvalidPadding         = newError("invalid padding")
	errInvalidStreamID        = newError("invalid streamID")
	errDecodeHeaderFrame      = newError("decode Header frame failed")
	errDecodePushPromiseFrame = newError("decode PushPromise frame failed")
)

// 在 HTTP/2 请求中 必须包含以下伪头部
//
// RFC 7540:
//  All HTTP/2 requests MUST include exactly one valid value for the :method, :scheme, and :path pseudo-header fields,
//  unless it is a CONNECT request [...] The :authority pseudo-header field MAY be omitted [...]
//  if the target URI includes an authority component.
//
// :method	定义 HTTP 方法（如 GET、POST）
// :scheme	定义协议类型（如 http、https）
// :path	定义请求路径和查询参数（如 /index/users?page=1）
//
// 与此同时 还有一个伪头部为可选项 但推荐上报
// :authority（可选）：替代 HTTP/1.1 的 Host 头 包含域名和端口（如 example.com:8080）
//
// 另外 伪头部字段必须位于常规头部字段之前 名称必须为小写且禁止重复

const (
	headerMethod    = ":method"
	headerScheme    = ":scheme"
	headerPath      = ":path"
	headerAuthority = ":authority"
	headerStatus    = ":status"
)

// HTTP/2 标准定义的帧类型如下
//
// * DATA Frame: 传输流的应用数据
// * HEADERS Frame: 传输头部信息 一般用于发起新流
// * PRIORITY Frame: 指定或重新指定流的优先级
// * RST_STREAM Frame: 终止流
// * SETTINGS Frame: 协商连接级参数
// * PUSH_PROMISE Frame: 服务器向客户端表明将发起流
// * PING Frame: 测量往返时间 检查连接活性
// * GOAWAY Frame: 通知对端不再接受新流
// * WINDOW_UPDATE Frame 实现流量控制 调整窗口大小
// * CONTINUATION Frame: 继续传输因单个 HEADERS 或 PUSH_PROMISE 帧无法容纳的头部块

const (
	frameData         = 0x0
	frameHeaders      = 0x1
	framePriority     = 0x2
	frameRSTStream    = 0x3
	frameSettings     = 0x4
	framePushPromise  = 0x5
	framePing         = 0x6
	frameGoAway       = 0x7
	frameWindowUpdate = 0x8
	frameContinuation = 0x9
)

const (
	// flagEndStream 用于 DATA 和 HEADERS 帧 表示当前是流的最后一帧
	// - DATA: 携带应用数据的最后片段
	// - HEADERS: 头部传输结束，后续不会发送帧内容
	flagEndStream = 0x1

	// flagEndHeaders 用于 HEADERS/PUSH_PROMISE/CONTINUATION 帧
	// 表示完整的头部块已传输完毕
	// - HEADERS/PUSH_PROMISE: 单个帧可容纳完整头部时设置
	// - CONTINUATION: 头部块分片的最后一帧必须设置
	flagEndHeaders = 0x4

	// flagPadded 用于 DATA/HEADERS/PUSH_PROMISE 帧
	// 表示帧包含填充数据（Pad Length + 填充字节）
	// 填充长度由帧首部后的第一个字节决定
	flagPadded = 0x8

	// flagPriority 用于 HEADERS 帧 表示包含优先级信息
	// 当设置时，帧负载会包含 31 位 Stream Dependency + 1 位 Exclusive 标志 + 8 位 Weight
	flagPriority = 0x20
)

const (
	// headerLength HTTP/2 标准定义的头部长度
	headerLength = 9
)

// streamDecoder stream 解析器
//
// HTTP/2 支持在同个 TCP 链接中同时发送处理多条数据流 则 Decoder 维护的最小状态
// 则应该为 Stream 本身 上层负责 `正确地` 拆分每个流的数据包并选择对应的 streamDecoder 进行 Decode
type streamDecoder struct {
	id         uint32
	t0         time.Time
	st         socket.TupleRaw
	serverPort socket.Port

	state     state
	frameType uint8
	flags     uint8

	payloadLen          uint32
	payloadConsumed     uint32
	lastPayloadConsumed uint32

	header        *HeaderFields
	headerDecoder *HeaderFieldDecoder
	headerBuf     *bytes.Buffer

	drainBytes int
	end        bool
	reqTime    time.Time
}

func newStreamDecoder(id uint32, st socket.TupleRaw, serverPort socket.Port, hfd *HeaderFieldDecoder) *streamDecoder {
	return &streamDecoder{
		id:            id,
		st:            st,
		headerBuf:     bufpool.Acquire(),
		serverPort:    serverPort,
		headerDecoder: hfd,
	}
}

// End 返回 Stream 是否结束 调用方需要负责管理 Stream 的生命周期且
// 在 Stream 关闭时调用 Free 清理资源
func (sd *streamDecoder) End() bool {
	return sd.end
}

// Free 释放 Decoder 资源
func (sd *streamDecoder) Free() {
	bufpool.Release(sd.headerBuf)
}

// Decode 解析 HTTP/2 数据包 当且仅当 Request / Response 结束时归档 Object
func (sd *streamDecoder) Decode(cut bool, b []byte, t time.Time) (*role.Object, error) {
	sd.t0 = t

	// 如果是被切割的状态 则修正状态 不进入 header 解析
	// 且同时需要复原上一轮的 payloadConsumed
	if cut {
		sd.state = stateDecodePayload
		sd.payloadConsumed = sd.lastPayloadConsumed
		sd.lastPayloadConsumed = 0
	}

	// 解析 header 获取 payload / flags 等信息
	if sd.state == stateDecodeHeader {
		if len(b) < headerLength {
			return nil, errDecodeHeader
		}
		err := sd.decodeHeader(b[:headerLength])
		if err != nil {
			return nil, err
		}
		b = b[headerLength:] // 切割剩余数据
	}

	complete, err := sd.decodePayload(cut, b)
	if err != nil {
		sd.reset()
		return nil, err
	}
	if complete {
		return sd.archive(), nil
	}
	return nil, nil
}

// isClient 判断是否为客户端
func (sd *streamDecoder) isClient() bool {
	return uint16(sd.serverPort) == sd.st.DstPort
}

// reset 重置状态
func (sd *streamDecoder) reset() {
	sd.state = stateDecodeHeader
	sd.frameType = 0
	sd.headerBuf.Reset()
	sd.header = nil
	sd.payloadLen = 0
	sd.payloadConsumed = 0
	sd.drainBytes = 0
	sd.flags = 0
}

// archive 归档请求
func (sd *streamDecoder) archive() *role.Object {
	if sd.header == nil {
		return nil
	}
	if sd.isClient() {
		field, hdr := sd.header.RequestHeader()
		obj := role.NewRequestObject(&Request{
			StreamID:  sd.id,
			Proto:     PROTO,
			Host:      sd.st.SrcIP,
			Port:      sd.st.SrcPort,
			Method:    field.Method,
			Scheme:    field.Scheme,
			Path:      field.Path,
			Authority: field.Authority,
			Header:    hdr,
			Size:      sd.drainBytes,
			Time:      sd.reqTime,
		})
		sd.reset()
		return obj
	}

	field, hdr := sd.header.ResponseHeader()
	obj := role.NewResponseObject(&Response{
		StreamID: sd.id,
		Proto:    PROTO,
		Host:     sd.st.SrcIP,
		Port:     sd.st.SrcPort,
		Status:   field.Status,
		Header:   hdr,
		Size:     sd.drainBytes,
		Time:     sd.t0,
	})
	sd.reset()
	return obj
}

// decodePayload payload 解编码入口
//
// 根据 header 中解析出的 FrameType 进行数据解析
func (sd *streamDecoder) decodePayload(cut bool, b []byte) (bool, error) {
	switch sd.frameType {
	case frameData:
		return sd.decodeDataFrame(cut, b)

	case frameHeaders:
		return sd.decodeHeaderFrame(b)

	case framePushPromise:
		return sd.decodePushPromise(b)

	case frameContinuation:
		return sd.decodeContinuationFrame(b)

	case frameRSTStream:
		return sd.decodeRstStreamFrame(b)

	case framePriority, frameSettings, framePing, frameGoAway, frameWindowUpdate:
		return sd.decodeTheRestFrames(b)
	}
	return false, errInvalidBytes // 切割数据包的时候出问题了 提前终止
}

// decodeHeader 解析 Header 固定 9 字节 布局如下
//
// +-----------------------------------------------+
// |                 Length (24)                   |
// +---------------+---------------+---------------+
// |   Type (8)    |   Flags (8)   |
// +-+-------------+---------------+-------------------------------+
// |R|                 Stream Identifier (31)                      |
// +-+-------------------------------------------------------------+
// |                   Frame Payload (0...)                      ...
// +---------------------------------------------------------------+
//
// * Length (24 bits): 帧负载的长度（不包括 9 字节头部）
// * Type (8 bits): 帧类型（如 0x0=DATA，0x1=HEADERS 等）
// * Flags (8 bits): 帧标志（如 END_STREAM、PADDED 等）
// * R (1 bit): 保留位 必须为 0
// * Stream Identifier (31 bits): 流标识符（0 表示与整个连接相关 如控制帧）
// * Frame Payload: 具体帧类型的负载数据
func (sd *streamDecoder) decodeHeader(b []byte) error {
	sd.drainBytes += len(b)
	payloadLen := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
	if payloadLen > maxPayloadSize {
		return errDecodeHeader
	}

	sd.state = stateDecodePayload
	sd.frameType = b[3]
	sd.flags = b[4]
	sd.payloadLen = payloadLen
	return nil
}

// decodeHeaderFrame 解析 HeaderFrame 帧布局如下
//
// +---------------+
// |Pad Length? (8)|
// +-+-------------+-----------------------------------------------+
// |E|                 Stream Dependency? (31)                     |
// +-+-------------+-----------------------------------------------+
// |  Weight? (8)  |
// +---------------+-----------------------------------------------+
// |                   Header Block Fragment (*)                 ...
// +---------------------------------------------------------------+
// |                           Padding (*)                       ...
// +---------------------------------------------------------------+
//
// * Pad Length/Padding (8 bits): 同 DATA 帧
// * E (1 bit): 流依赖的独占标志
// * Stream Dependency (31 bits): 依赖的流 ID（用于优先级）
// * Weight (8 bits): 流的权重（1~256）
// * Header Block Fragment: HPACK 压缩后的头部块
// * Padding: 填充块
//
// GRPC 协议中还声明了 Trailer 机制 基于 HTTP/2 协议 其响应分为两个部分
//
// - 响应头 (Headers): 在响应开始时发送 通常包含元数据（metadata）
// - 尾部头 (Trailers): 在响应结束时发送 必须包含 grpc-status 和 grpc-message 用于标识 RPC 的最终状态
//
// 即如果观察到两次 HEADERS 帧
//
// - 第一次 正常的响应头（Headers）
// - 第二次 尾部头（Trailers）包含 gRPC 状态码
//
// Client                            Server
//
//	|       HEADERS (Request)          |
//	| -------------------------------> |
//	|       DATA (Request Body)        |
//	| -------------------------------> |
//	|      HEADERS (Response Headers)  | -- 第一次 Headers（可能含 metadata）
//	| <------------------------------- |
//	|       DATA (Response Body)       |
//	| <------------------------------- |
//	|       HEADERS (Trailers)         | -- 第二次 Headers（必须含 grpc-status）
//	| <------------------------------- |
//
// 这种情况下也需要将 Stream 标记为 end 状态
func (sd *streamDecoder) decodeHeaderFrame(b []byte) (bool, error) {
	sd.drainBytes += len(b)
	n := sd.payloadConsumed + uint32(len(b))

	// 读取到的并非完整 Header 数据包
	if n < sd.payloadLen {
		return false, errDecodeHeaderFrame
	}

	// 超过了 Header 要求的字节数 按需保留
	// 正常情况下是不会超过的
	if n >= sd.payloadLen {
		consumed := sd.payloadLen - sd.payloadConsumed
		b = b[:consumed]
	}

	// Padded Flag 需要剔除尾部的填充项
	if sd.flags&flagPadded != 0 {
		if len(b) < 1 {
			return false, errInvalidPadding
		}
		padLen := int(b[0])
		if padLen >= len(b) {
			return false, errInvalidPadding
		}
		b = b[1 : len(b)-padLen]
		sd.payloadConsumed += uint32(padLen) + 1
	}

	sd.payloadConsumed += uint32(len(b))

	// Priority Flag 需要剔除接下来的 4 字节
	if sd.flags&flagPriority != 0 {
		if len(b) < 5 {
			return false, errInvalidBytes
		}
		b = b[5:]
	}

	sd.headerBuf.Write(b)

	// header 已经结束 另外一种情况则是需要在另外的 ContinuationFrame 继续追加 header
	var isTrailers bool
	if sd.flags&flagEndHeaders != 0 {
		newHdr := sd.headerDecoder.Decode(sd.headerBuf.Bytes())
		isTrailers = newHdr.IsTrailers()
		if !isTrailers {
			sd.header = newHdr
		}
		sd.headerBuf.Reset() // 复用 buffer
		sd.reqTime = sd.t0   // Header 帧确定后即标记为请求开始时间
		sd.payloadConsumed = 0
		sd.payloadLen = 0
	}

	sd.state = stateDecodeHeader
	sd.end = sd.flags&flagEndStream != 0

	if isTrailers {
		return true, nil
	}
	return false, nil
}

// decodeContinuationFrame 解析 ContinuationFrame 帧布局如下
//
// +---------------------------------------------------------------+
// |                   Header Block Fragment (*)                 ...
// +---------------------------------------------------------------+
//
// ContinuationFrame 用于延续未发送完的 HEADERS 或 PUSH_PROMISE 帧的头部块
// 如果在本帧内 Header 还为结束 则需要等待下一个数据包
func (sd *streamDecoder) decodeContinuationFrame(b []byte) (bool, error) {
	sd.drainBytes += len(b)

	var isTrailers bool
	sd.headerBuf.Write(b)
	if sd.flags&flagEndHeaders != 0 {
		newHdr := sd.headerDecoder.Decode(sd.headerBuf.Bytes())
		isTrailers = newHdr.IsTrailers()
		if !isTrailers {
			sd.header = newHdr
		}
		sd.headerBuf.Reset()
		sd.reqTime = sd.t0 // Header 帧确定后即标记为请求开始时间
		sd.payloadConsumed = 0
		sd.payloadLen = 0
	}

	sd.state = stateDecodeHeader
	sd.end = sd.flags&flagEndStream != 0
	return false, nil
}

// decodeDataFrame 解析 DataFrame 帧布局如下
//
// +---------------+
// |Pad Length? (8)|
// +---------------+-----------------------------------------------+
// |                            Data (*)                         ...
// +---------------------------------------------------------------+
// |                           Padding (*)                       ...
// +---------------------------------------------------------------+
//
// * Pad Length/Padding (8 bits): 可选字段 仅当设置了 PADDED 标志时存在
// * Data: 实际数据
// * Padding：填充字节（长度由 Pad Length 指定）
func (sd *streamDecoder) decodeDataFrame(cut bool, b []byte) (bool, error) {
	sd.drainBytes += len(b)

	// 当 DataFrame 在上一层被切割以后 可能会出现分多个包传入解析的情况
	// 此时只有第一个包需要 padded
	if sd.flags&flagPadded != 0 && !cut {
		if len(b) < 1 {
			return false, errInvalidPadding
		}
		padLen := int(b[0])
		if padLen >= len(b) {
			return false, errInvalidPadding
		}
		b = b[1 : len(b)-padLen]
		sd.payloadConsumed += uint32(padLen) + 1
	}

	sd.payloadConsumed += uint32(len(b))
	sd.end = sd.flags&flagEndStream != 0
	complete := sd.payloadLen == sd.payloadConsumed

	// 必须要 stream 结束才标记为完成
	if complete && sd.end {
		return true, nil
	}

	sd.lastPayloadConsumed = sd.payloadConsumed // 避免 payload 被清零后算 complete 出错
	sd.payloadConsumed = 0
	sd.state = stateDecodeHeader
	return false, nil
}

// decodeRstStreamFrame 解析 RstStreamFrame 帧布局如下
//
// +---------------------------------------------------------------+
// |                        Error Code (32)                        |
// +---------------------------------------------------------------+
//
// Error Code (32 bits): 错误码（如 NO_ERROR(0x0)、PROTOCOL_ERROR(0x1) 等）
//
// 当客户端或服务器发现流的处理出现不可恢复的错误（如协议错误、请求被取消等）出现
// 解析到此帧时需要关闭 Stream
func (sd *streamDecoder) decodeRstStreamFrame(b []byte) (bool, error) {
	sd.end = true
	return false, nil
}

// decodePushPromise 解析 PushPromise 帧布局如下
//
// +---------------+
// |Pad Length? (8)|
// +-+-------------+-----------------------------------------------+
// |R|                  Promised Stream ID (31)                    |
// +-+-----------------------------+-------------------------------+
// |                   Header Block Fragment (*)                 ...
// +---------------------------------------------------------------+
// |                           Padding (*)                       ...
// +---------------------------------------------------------------+
//
// * Pad Length/Padding (8 bits): 同 DATA 帧
// * R (1 bit): 保留位
// * Promised Stream ID: StreamID
// * Weight (8 bits): 流的权重（1~256）
// * Header Block Fragment: HPACK 压缩后的头部块
// * Padding: 填充块
func (sd *streamDecoder) decodePushPromise(b []byte) (bool, error) {
	sd.drainBytes += len(b)
	n := sd.payloadConsumed + uint32(len(b))

	// 读取到的并非完整 Header 数据包
	if n < sd.payloadLen {
		return false, errDecodePushPromiseFrame
	}

	// Padded Flag 需要剔除尾部的填充项
	if sd.flags&flagPadded != 0 {
		if len(b) < 1 {
			return false, errInvalidPadding
		}
		padLen := int(b[0])
		if padLen >= len(b) {
			return false, errInvalidPadding
		}
		b = b[1 : len(b)-padLen]
		sd.payloadConsumed += uint32(padLen) + 1
	}

	sd.payloadConsumed += uint32(len(b))
	if len(b) < 4 {
		return false, errInvalidStreamID
	}

	b = b[4:] // StreamID
	sd.headerBuf.Write(b)

	// PushPromise 帧使用 flagEndHeaders 来判断是否结束
	// 其本质与 HeaderFrame 类似
	var isTrailers bool
	if sd.flags&flagEndHeaders != 0 {
		newHdr := sd.headerDecoder.Decode(sd.headerBuf.Bytes())
		isTrailers = newHdr.IsTrailers()
		if !isTrailers {
			sd.header = newHdr
		}
		sd.headerBuf.Reset() // 复用 buffer
		sd.reqTime = sd.t0   // PushPromise 帧确定后即标记为请求开始时间
		sd.payloadConsumed = 0
		sd.payloadLen = 0
	}

	sd.state = stateDecodeHeader
	sd.end = sd.flags&flagEndStream != 0
	return false, nil // PushPromise 帧肯定不会是请求的结束标识
}

// decodeTheRestFrames 解析剩余的非重要的数据帧
func (sd *streamDecoder) decodeTheRestFrames(b []byte) (bool, error) {
	sd.drainBytes += len(b)
	sd.state = stateDecodeHeader
	return false, nil
}
