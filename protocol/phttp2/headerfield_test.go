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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeaderFieldDecoder(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  map[string]string
	}{
		{
			name:  "EmptyInput",
			input: []byte{},
			want:  map[string]string{},
		},
		{
			name: "SingleHeader",
			input: []byte{
				0x40, 0x0c, 'c', 'o', 'n', 't', 'e', 'n', 't', '-', 't', 'y', 'p', 'e',
				0x10, 'a', 'p', 'p', 'l', 'i', 'c', 'a', 't', 'i', 'o', 'n', '/', 'j', 's', 'o', 'n',
			},
			want: map[string]string{
				"content-type": "application/json",
			},
		},
		{
			name: "MultipleHeaders",
			input: []byte{
				0x82,
				0x44, 0x0b, '/', 'i', 'n', 'd', 'e', 'x', '.', 'h', 't', 'm', 'l',
				0x40, 0x0a, 'u', 's', 'e', 'r', '-', 'a', 'g', 'e', 'n', 't',
				0x0b, 't', 'e', 's', 't', '-', 'c', 'l', 'i', 'e', 'n', 't',
			},
			want: map[string]string{
				":method":    "GET",
				":path":      "/index.html",
				"user-agent": "test-client",
			},
		},
		{
			name: "DuplicateKeys",
			input: []byte{
				0x40, 0x09, 'x', '-', 'v', 'e', 'r', 's', 'i', 'o', 'n',
				0x03, '1', '.', '0',
				0x40, 0x09, 'x', '-', 'v', 'e', 'r', 's', 'i', 'o', 'n',
				0x03, '2', '.', '0',
			},
			want: map[string]string{
				"x-version": "2.0",
			},
		},
		{
			name: "BinaryData",
			input: []byte{
				0x40, 0x0b, 'b', 'i', 'n', 'a', 'r', 'y', '-', 'd', 'a', 't', 'a',
				0x04, 0x00, 0x01, 0x7f, 0xff,
			},
			want: map[string]string{
				"binary-data": string([]byte{0x00, 0x01, 0x7f, 0xff}),
			},
		},
		{
			name: "CaseSensitiveKeys",
			input: []byte{
				0x40, 0x0c, 'C', 'o', 'n', 't', 'e', 'n', 't', '-', 'T', 'y', 'p', 'e',
				0x0a, 't', 'e', 'x', 't', '/', 'p', 'l', 'a', 'i', 'n',
				0x40, 0x0c, 'c', 'o', 'n', 't', 'e', 'n', 't', '-', 't', 'y', 'p', 'e',
				0x10, 'a', 'p', 'p', 'l', 'i', 'c', 'a', 't', 'i', 'o', 'n', '/', 'j', 's', 'o', 'n',
			},
			want: map[string]string{
				"content-type": "application/json",
				"Content-Type": "text/plain",
			},
		},
		{
			name: "MixedStaticAndDynamic",
			input: []byte{
				0x82, 0x85, 0x40, 0x04, 'h', 'o', 's', 't',
				0x09, 'l', 'o', 'c', 'a', 'l', 'h', 'o', 's', 't',
			},
			want: map[string]string{
				":path":   "/index.html",
				":method": "GET",
				"host":    "localhost",
			},
		},
		{
			name: "PseudoHeaderOrderValidation",
			input: []byte{
				0x40, 0x05, 'v', 'a', 'l', 'u', 'e',
				0x03, '1', '2', '3',
				0x82,
			},
			want: map[string]string{
				":method": "GET",
				"value":   "123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := NewHeaderFieldDecoder()
			defer dec.Release()
			fields := dec.Decode(tt.input)
			assert.Equal(t, tt.want, fields.fields)
		})
	}
}
