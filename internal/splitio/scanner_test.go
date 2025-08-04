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
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanner(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  [][]byte
	}{
		{
			name:  "EmptyInput",
			input: []byte{},
			want:  nil,
		},
		{
			name:  "SingleLineWithoutLF",
			input: []byte("hello world"),
			want: [][]byte{
				[]byte("hello world"),
			},
		},
		{
			name:  "SingleLineWithLF",
			input: []byte("hello\n"),
			want: [][]byte{
				[]byte("hello\n"),
			},
		},
		{
			name:  "MultipleLines",
			input: []byte("line1\nline2\nline3\n"),
			want: [][]byte{
				[]byte("line1\n"),
				[]byte("line2\n"),
				[]byte("line3\n"),
			},
		},
		{
			name:  "MixedLineEndings",
			input: []byte("unix\nwindows\r\nmac\r"),
			want: [][]byte{
				[]byte("unix\n"),
				[]byte("windows\r\n"),
				[]byte("mac\r"),
			},
		},
		{
			name:  "ConsecutiveLFs",
			input: []byte("\n\n\n\n"),
			want: [][]byte{
				[]byte("\n"),
				[]byte("\n"),
				[]byte("\n"),
				[]byte("\n"),
			},
		},
		{
			name:  "EmbeddedLFs",
			input: []byte("foo\x00\nbar\x1b\nbaz"),
			want: [][]byte{
				[]byte("foo\x00\n"),
				[]byte("bar\x1b\n"),
				[]byte("baz"),
			},
		},
		{
			name:  "CRLFTerminated",
			input: []byte("line1\r\nline2\r\n"),
			want: [][]byte{
				[]byte("line1\r\n"),
				[]byte("line2\r\n"),
			},
		},
		{
			name:  "MixedEmptyLines",
			input: []byte("\n\nhello\n\nworld\n\n"),
			want: [][]byte{
				[]byte("\n"),
				[]byte("\n"),
				[]byte("hello\n"),
				[]byte("\n"),
				[]byte("world\n"),
				[]byte("\n"),
			},
		},
		{
			name:  "BinaryData",
			input: []byte{0x00, 0x0A, 0xFF, 0x0A, 0x0D},
			want: [][]byte{
				{0x00, 0x0A},
				{0xFF, 0x0A},
				{0x0D},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := NewScanner(tt.input)
			var lines [][]byte
			for scanner.Scan() {
				lines = append(lines, scanner.Bytes())
			}
			assert.Equal(t, tt.want, lines)
		})
	}
}

type mockScanner interface {
	Scan() bool
	Bytes() []byte
}

func BenchmarkZeroCopyScanner(b *testing.B) {
	benchmarkScanner(b, func(b []byte) mockScanner {
		return NewScanner(b)
	})
}

func BenchmarkBufioScanner(b *testing.B) {
	benchmarkScanner(b, func(b []byte) mockScanner {
		return bufio.NewScanner(bytes.NewBuffer(b))
	})
}

func benchmarkScanner(b *testing.B, f func([]byte) mockScanner) {
	var input []byte
	input = append(input, bytes.Repeat([]byte(strings.Repeat("x", 1024)+"\n"), 100)...)

	b.ReportAllocs()
	b.ResetTimer()
	b.SetBytes(int64(len(input)))

	b.RunParallel(func(pb *testing.PB) {
		var n int
		for pb.Next() {
			scanner := f(input)
			for scanner.Scan() {
				line := scanner.Bytes()
				n += len(line)
			}
		}
	})
}
