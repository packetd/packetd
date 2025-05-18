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

package splitio

import (
	"bytes"
)

var (
	CharCRLF = []byte("\r\n")
	CharCR   = []byte("\r")
	CharLF   = []byte("\n")
)

type Scanner struct {
	l, r int
	buf  []byte
}

// NewScanner 创建并返回 *Scanner 实例
//
// 需要保留切割后的换行符 `\r\n` 或者 `\n`
// 此版本会比 *bufio.Scanner 性能更高 参见 Benchmark
// 后者会拷贝 buf 内容造成额外的开销
func NewScanner(b []byte) *Scanner {
	return &Scanner{
		buf: b,
	}
}

// Scan 扫描下一个 LR 字符并标记索引
func (s *Scanner) Scan() bool {
	s.l = s.r
	if len(s.buf) == s.l {
		return false
	}

	idx := bytes.IndexByte(s.buf[s.l:], CharLF[0])
	if idx == -1 {
		s.r = len(s.buf)
	} else {
		s.r = s.l + idx + 1
	}
	return true
}

// Bytes 读取下一行 如有修改需求 请拷贝一份
func (s *Scanner) Bytes() []byte {
	return s.buf[s.l:s.r]
}
