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

package connstream

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/internal/splitio"
	"github.com/packetd/packetd/internal/zerocopy"
)

func TestWriteChunk(t *testing.T) {
	tests := []struct {
		name        string
		input       [][]byte
		endWithCRLF []bool
		total       int
	}{
		{
			name: "CRLF at end",
			input: [][]byte{
				bytes.Repeat([]byte("a"), common.ReadWriteBlockSize-68),
				bytes.Repeat([]byte("a"), 3),
				splitio.CharCRLF,
			},
			endWithCRLF: []bool{true},
			total:       common.ReadWriteBlockSize - 63,
		},
		{
			name: "CRLF at end",
			input: [][]byte{
				bytes.Repeat([]byte("z"), common.ReadWriteBlockSize-2),
				splitio.CharCRLF,
			},
			endWithCRLF: []bool{true},
			total:       common.ReadWriteBlockSize,
		},
		{
			name: "CRLF split across chunks",
			input: [][]byte{
				bytes.Repeat([]byte("a"), common.ReadWriteBlockSize-70),
				splitio.CharCRLF,
				bytes.Repeat([]byte("b"), 50),
			},
			endWithCRLF: []bool{false, false},
			total:       common.ReadWriteBlockSize - 18,
		},
		{
			name: "CR only in middle",
			input: [][]byte{
				bytes.Repeat([]byte("a"), common.ReadWriteBlockSize-100),
				{'C', '\r', 'M', 'D'},
				bytes.Repeat([]byte("b"), 100),
			},
			endWithCRLF: []bool{false, false},
			total:       common.ReadWriteBlockSize + 4,
		},
		{
			name: "LF only at boundary",
			input: [][]byte{
				bytes.Repeat([]byte("x"), common.ReadWriteBlockSize-1),
				splitio.CharLF,
				[]byte("trailing-data"),
			},
			endWithCRLF: []bool{false, false},
			total:       common.ReadWriteBlockSize + 13,
		},
		{
			name: "Multiple CRLF in a chunk",
			input: [][]byte{
				[]byte("data"),
				splitio.CharCRLF,
				bytes.Repeat([]byte("a"), 500),
				splitio.CharCRLF,
				bytes.Repeat([]byte("b"), 500),
			},
			endWithCRLF: []bool{false},
			total:       1008,
		},
		{
			name: "CRLF at beginning",
			input: [][]byte{
				splitio.CharCRLF,
				[]byte("payload"),
			},
			endWithCRLF: []bool{false},
			total:       9,
		},
		{
			name: "Mixed CR/LF/CRLF",
			input: [][]byte{
				[]byte("CR:\r LF:\n CRLF:\r\n"),
				[]byte("END\r"),
				[]byte("\n"),
			},
			endWithCRLF: []bool{true},
			total:       22,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cw := newChunkWriter()
			var total int
			var endWithCRLF []bool
			f := func(r zerocopy.Reader) {
				b, _ := r.Read(common.ReadWriteBlockSize)
				total += len(b)
				endWithCRLF = append(endWithCRLF, bytes.HasSuffix(b, splitio.CharCRLF))
			}

			var b bytes.Buffer
			for _, payload := range tt.input {
				b.Write(payload)
			}
			cw.Write(b.Bytes(), f)
			assert.Equal(t, tt.total, total)
			assert.Equal(t, tt.endWithCRLF, endWithCRLF)
		})
	}
}
