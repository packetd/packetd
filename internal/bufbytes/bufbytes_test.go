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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBufBytesWrite(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		inputs   [][]byte
		expected []byte
	}{
		{
			name:     "Empty write",
			size:     10,
			inputs:   [][]byte{},
			expected: nil,
		},
		{
			name:     "Single fit",
			size:     5,
			inputs:   [][]byte{[]byte("hello")},
			expected: []byte("hello"),
		},
		{
			name:     "Single write within capacity",
			size:     10,
			inputs:   [][]byte{[]byte("hello")},
			expected: []byte("hello"),
		},
		{
			name:     "Single write exceeds capacity",
			size:     5,
			inputs:   [][]byte{[]byte("helloworld")},
			expected: []byte("hello"),
		},
		{
			name:     "Multiple inputs within capacity",
			size:     10,
			inputs:   [][]byte{[]byte("hello"), []byte("world")},
			expected: []byte("helloworld"),
		},
		{
			name:     "Multiple inputs exceed capacity",
			size:     8,
			inputs:   [][]byte{[]byte("hello"), []byte("world")},
			expected: []byte("hellowor"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(tt.size)
			for _, input := range tt.inputs {
				b.Write(input)
			}
			assert.Equal(t, tt.expected, b.buf)
		})
	}
}
