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

package bufbytes

import "bytes"

const (
	cStringEnd = '\x00'
)

type Bytes struct {
	size int
	buf  []byte
}

func New(size int) *Bytes {
	return &Bytes{
		size: size,
	}
}

func (b *Bytes) Write(p []byte) {
	n := (b.size - len(b.buf)) - len(p)
	if n >= 0 {
		b.buf = append(b.buf, p...)
		return
	}

	l := b.size - len(b.buf)
	if l > 0 {
		b.buf = append(b.buf, p[:l]...)
	}
}

func (b *Bytes) Len() int {
	return len(b.buf)
}

func (b *Bytes) Text() string {
	return string(b.buf)
}

func (b *Bytes) TrimCStringText() string {
	if !bytes.HasSuffix(b.buf, []byte{cStringEnd}) {
		return b.Text()
	}
	return string(b.buf[:len(b.buf)-1])
}

func (b *Bytes) Clone() []byte {
	if b.buf == nil {
		return nil
	}
	return append([]byte{}, b.buf...)
}

func (b *Bytes) Reset() {
	b.buf = b.buf[:0]
}
