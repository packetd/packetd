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

package zerocopy

import (
	"io"
)

// Reader ZeroCopy-API
//
// Reader Read 零拷贝方式读取 n 字节数据
type Reader interface {
	Read(n int) ([]byte, error)
}

// Writer ZeroCopy-API
//
// Writer Write 零拷贝方式写入数据 写入不会失败
type Writer interface {
	Write(p []byte)
}

// Closer ZeroCopy-API
//
// Close 将 Reader 置为 io.EOF 状态
type Closer interface {
	Close()
}

// Buffer ZeroCopy-API
//
// 支持 Write/Read/Close 方法 次接口的所有操作均为零拷贝
type Buffer interface {
	Writer
	Reader
	Closer
}

type buffer struct {
	r int
	b []byte
}

// NewBuffer 创建并返回 Buffer 实例
//
// 此实现只有在 tcpstream 的写入场景下使用
// 可以避免拷贝从网卡中读取的数据 但前提条件是使用此接口的调用方 `不修改任何字节数据`
//
// Write 写入性能会由于 bytes.Buffer Write 实现 参见 benchmark
func NewBuffer(p []byte) Buffer {
	return &buffer{
		b: p,
	}
}

// Read 实现 Reader 接口
func (buf *buffer) Read(n int) ([]byte, error) {
	if buf.r == len(buf.b) {
		return nil, io.EOF
	}

	if buf.r+n >= len(buf.b) {
		b := buf.b[buf.r:len(buf.b)]
		buf.r = len(buf.b)
		return b, nil
	}

	b := buf.b[buf.r : buf.r+n]
	buf.r += n
	return b, nil
}

// Write 实现 Writer 接口
func (buf *buffer) Write(p []byte) {
	buf.b = p
	buf.r = 0
}

// Close 实现 Close 接口
func (buf *buffer) Close() {
	buf.r = len(buf.b)
}
