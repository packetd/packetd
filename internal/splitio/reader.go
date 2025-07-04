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

type Reader struct {
	r, w    int
	b       []byte
	scanner *Scanner
}

// NewReader 创建并返回 *Reader 实例
//
// 需要保留切割后的换行符 `\r\n` 或者 `\n`
// 此版本会比 *bufio.Reader 性能更高 参见 Benchmark
// 后者会拷贝 buf 内容造成额外的开销
func NewReader(b []byte) *Reader {
	return &Reader{
		w:       len(b),
		b:       b,
		scanner: NewScanner(b),
	}
}

// ReadLine 程序读取一行数据
func (lr *Reader) ReadLine() ([]byte, bool) {
	if !lr.scanner.Scan() {
		return nil, true // EOF
	}

	b := lr.scanner.Bytes()
	lr.r += len(b)
	return b, false
}

// EOF 返回 Reader 是否已到达 EOF
func (lr *Reader) EOF() bool {
	return lr.r >= lr.w
}
