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

package predis

import (
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol/role"
)

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *Request
	}{
		{
			name:  "Command SET",
			input: "*3\r\n$3\r\nSET\r\n$4\r\nkey1\r\n$5\r\nvalue\r\n",
			want: &Request{
				Command: "SET",
				Size:    12,
			},
		},
		{
			name:  "Command GET",
			input: "*2\r\n$3\r\nGET\r\n$4\r\nkey1\r\n",
			want: &Request{
				Command: "GET",
				Size:    7,
			},
		},
		{
			name:  "Command HSET",
			input: "*6\r\n$4\r\nHSET\r\n$4\r\nhash\r\n$6\r\nfield1\r\n$6\r\nvalue1\r\n$6\r\nfield2\r\n$6\r\nvalue2\r\n",
			want: &Request{
				Command: "HSET",
				Size:    32,
			},
		},
		{
			name:  "Command ZADD",
			input: "*6\r\n$4\r\nZADD\r\n$3\r\nkey\r\n$1\r\n1\r\n$7\r\nmember1\r\n$1\r\n2\r\n$7\r\nmember2\r\n",
			want: &Request{
				Command: "ZADD",
				Size:    23,
			},
		},
		{
			name:  "Command MSET",
			input: "*5\r\n$4\r\nMSET\r\n$4\r\nkey1\r\n$4\r\nval1\r\n$4\r\nkey2\r\n$4\r\nval2\r\n",
			want: &Request{
				Command: "MSET",
				Size:    20,
			},
		},
		{
			name:  "Command LLEN",
			input: "*2\r\n$4\r\nLLEN\r\n$6\r\nmylist\r\n*3\r\n$4\r\nSADD\r\n$4\r\nmyset\r\n$6\r\nmember\r\n",
			want: &Request{
				Command: "LLEN",
				Size:    10,
			},
		},
		{
			name:  "Command CLIENT",
			input: "*2\r\n$6\r\nCLIENT\r\n$4\r\nINFO\r\n",
			want: &Request{
				Command: "CLIENT INFO",
				Size:    10,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0)
			objs, err := d.Decode(zerocopy.NewBuffer([]byte(tt.input)), t0)
			assert.NoError(t, err)

			obj := objs[0].Obj.(*Request)
			assert.Equal(t, tt.want.Command, obj.Command)
			assert.Equal(t, tt.want.Size, obj.Size)
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *Response
	}{
		{
			name:  "SimpleStrings OK",
			input: "+OK\r\n",
			want: &Response{
				DataType: string(SimpleStrings),
				Size:     2,
			},
		},
		{
			name:  "SimpleStrings PONG",
			input: "+PONG\r\n",
			want: &Response{
				DataType: string(SimpleStrings),
				Size:     4,
			},
		},
		{
			name:  "Error simple error",
			input: "-Error message\r\n",
			want: &Response{
				DataType: string(Errors),
				Size:     13,
			},
		},
		{
			name:  "Error wrong type",
			input: "-WRONGTYPE Operation against a key holding the wrong kind of value\r\n",
			want: &Response{
				DataType: string(Errors),
				Size:     65,
			},
		},
		{
			name:  "Integers 1000",
			input: ":1000\r\n",
			want: &Response{
				DataType: string(Integers),
				Size:     4,
			},
		},
		{
			name:  "Integers -1000",
			input: ":-1000\r\n",
			want: &Response{
				DataType: string(Integers),
				Size:     5,
			},
		},
		{
			name:  "Integers maxInt64",
			input: ":9223372036854775807\r\n",
			want: &Response{
				DataType: string(Integers),
				Size:     19,
			},
		},
		{
			name:  "Integers minInt64",
			input: ":-9223372036854775808\r\n",
			want: &Response{
				DataType: string(Integers),
				Size:     20,
			},
		},
		{
			name:  "BulkStrings empty string",
			input: "$0\r\n\r\n",
			want: &Response{
				DataType: string(BulkStrings),
				Size:     0,
			},
		},
		{
			name:  "BulkStrings null",
			input: "$-1\r\n",
			want: &Response{
				DataType: string(BulkStrings),
				Size:     0,
			},
		},
		{
			name:  "BulkStrings with newline",
			input: "$11\r\nHello\nWorld\r\n",
			want: &Response{
				DataType: string(BulkStrings),
				Size:     11,
			},
		},
		{
			name:  "BulkStrings large string",
			input: "$1000\r\n" + strings.Repeat("a", 1000) + "\r\n",
			want: &Response{
				DataType: string(BulkStrings),
				Size:     1000,
			},
		},
		{
			name:  "Array empty",
			input: "*0\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     0,
			},
		},
		{
			name:  "Array null",
			input: "*-1\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     0,
			},
		},
		{
			name:  "Array all null elements",
			input: "*3\r\n$-1\r\n$-1\r\n$-1\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     0,
			},
		},
		{
			name: "Array UTF8",
			input: "*2\r\n" +
				"$6\r\n中文\r\n" +
				"*2\r\n$6\r\n测试\r\n:42\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     14,
			},
		},
		{
			name: "Array deep array",
			input: "*10\r\n" +
				strings.Repeat("*5\r\n:1\r\n:2\r\n:3\r\n:4\r\n:5\r\n", 9) +
				":end\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     48,
			},
		},
		{
			name: "Array nested",
			input: "*5\r\n" +
				":100\r\n" +
				"$-1\r\n" +
				"*3\r\n+OK\r\n-ERR\r\n:42\r\n" +
				"*2\r\n*2\r\n$5\r\nhello\r\n$5\r\nworld\r\n*1\r\n:99\r\n" +
				"$7\r\n\x00\xFF\xFE\xFD\xFC\xFB\xFA\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     29,
			},
		},
		{
			name: "Array nested",
			input: "*2\r\n" +
				"*3\r\n:0\r\n:5460\r\n*3\r\n$9\r\n127.0.0.1\r\n:7000\r\n$40\r\nc825edc8d68cdaad8e945da4f939d8a7e7e29e11\r\n" +
				"*3\r\n:5461\r\n:10922\r\n*3\r\n$9\r\n127.0.0.1\r\n:7001\r\n$40\r\n9bc7f47f09a7e35b9c9b614dc4aef13c6a44f6d6\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     120,
			},
		},
		{
			name: "Array nested",
			input: "*5\r\n" +
				":1\r\n" +
				"$-1\r\n" +
				"*3\r\n:1\r\n$-1\r\n:2\r\n" +
				"*-1\r\n" +
				"$0\r\n\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     3,
			},
		},
		{
			name: "Array nested",
			input: "*2\r\n" +
				"*2\r\n*2\r\n:1\r\n:2\r\n*2\r\n:3\r\n:4\r\n" +
				"*2\r\n*2\r\n$5\r\nhello\r\n$5\r\nworld\r\n*1\r\n-Error\r\n",
			want: &Response{
				DataType: string(Array),
				Size:     19,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0)
			objs, err := d.Decode(zerocopy.NewBuffer([]byte(tt.input)), t0)
			assert.NoError(t, err)

			obj := objs[0].Obj.(*Response)
			assert.Equal(t, tt.want.DataType, obj.DataType)
			assert.Equal(t, tt.want.Size, obj.Size)
		})
	}
}

func TestDecodeFailed(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:  "BulkStrings partial content",
			input: "$6\r\nfoo\r\n",
		},
		{
			name:  "Array partial content",
			input: "*2\r\n$3\r\nGET\r\n",
		},
		{
			name:    "Invalid first byte",
			input:   "invalid\r\n",
			wantErr: errInvalidBytes,
		},
		{
			name:    "Invalid number format",
			input:   "*abc\r\n",
			wantErr: errDecodeN,
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0)
			got, err := d.Decode(zerocopy.NewBuffer([]byte(tt.input)), t0)
			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr))
			} else {
				assert.NoError(t, err)
			}
			assert.Nil(t, got)
		})
	}
}

func TestDecodeMultiple(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
		want   *Response
	}{
		{
			name: "BulkStrings large string",
			inputs: []string{
				"$16384\r\n" + strings.Repeat("a", 4096-10),
				strings.Repeat("a", 4096),
				strings.Repeat("a", 4096),
				strings.Repeat("a", 4096),
				strings.Repeat("a", 10) + "\r\n",
			},
			want: &Response{
				DataType: string(BulkStrings),
				Size:     16384,
			},
		},
		{
			name: "BulkStrings large string",
			inputs: []string{
				"$40960\r\n" + strings.Repeat("a", 4096-10),
				strings.Repeat("b", 4096),
				strings.Repeat("c", 4096),
				strings.Repeat("d", 4096),
				strings.Repeat("e", 4096),
				strings.Repeat("f", 4096),
				strings.Repeat("g", 4096),
				strings.Repeat("h", 4096),
				strings.Repeat("i", 4096),
				strings.Repeat("j", 4096),
				strings.Repeat("k", 10) + "\r\n",
			},
			want: &Response{
				DataType: string(BulkStrings),
				Size:     40960,
			},
		},
		{
			name: "Array complex pipeline",
			inputs: []string{
				"*5\r\n:100\r\n$-1\r\n*3\r\n+OK\r\n-ERR\r\n:42\r\n*2\r\n*2\r\n$5\r\nhello\r\n$5\r\nwo",
				"rld\r\n*1\r\n:99\r\n$7\r\n\x00\xFF\xFE\xFD\xFC\xFB\xFA\r\n",
			},
			want: &Response{
				DataType: string(Array),
				Size:     29,
			},
		},
		{
			name: "Array complex pipeline",
			inputs: []string{
				"*5\r\n:100\r\n$-1\r\n*3\r\n+OK\r\n-ERR\r\n:42\r\n*2\r\n*2\r\n$5\r\nhe",
				"llo\r\n$5\r\nwo",
				"rld\r\n*1\r\n:99\r\n$7\r\n\x00\xFF\xFE\xFD\xFC\xFB\xFA\r\n",
			},
			want: &Response{
				DataType: string(Array),
				Size:     29,
			},
		},
		{
			name: "Array nested",
			inputs: []string{
				"*2\r\n*3\r\n:0\r\n:5460\r\n*3\r\n$9\r\n12",
				"7.0.0.1\r\n:7000\r\n$40\r\nc825edc8d68cdaad8e945da4f939d8a7e7e29e11\r\n*3\r\n:5461\r\n:10922\r\n*3\r\n$9\r\n127.0.0.1\r\n:7001\r\n$40\r\n9bc7f47f09a7e35b9c9b614dc4aef13c6a44f6d6\r\n",
			},
			want: &Response{
				DataType: string(Array),
				Size:     120,
			},
		},
		{
			name: "Array nested",
			inputs: []string{
				"*2\r\n*3\r\n:0\r\n:5460\r\n*3\r\n$9\r\n12",
				"7.0.0.1\r\n:7000\r\n$40\r\nc825edc8d68cdaad8e",
				"945da4f939d8a7e7e29e11\r\n*3\r\n:5461\r\n:10922\r\n*3\r\n$9\r\n127.",
				"0.0.1\r\n:7001\r\n$40\r\n9bc7f47f09a7e35b9c9b6",
				"14dc4aef13c6a44f6d6\r\n",
			},
			want: &Response{
				DataType: string(Array),
				Size:     120,
			},
		},
		{
			name: "Array nested",
			inputs: []string{
				"*2\r\n*3\r\n:0\r\n:5460\r\n*3\r\n$9\r\n12",
				"7.0.0.1\r\n:7000\r\n$40\r\nc825edc8d68cdaad8e",
				"945da4f939d8",
				"a7e7e29e11\r\n*3\r\n:5461\r\n:10922\r\n*3\r\n$9\r\n127.",
				"0.0.1\r\n:7001\r\n$40\r\n9bc7f47",
				"f09a7e35b9c9b6",
				"14dc4aef13c6a44f6d6\r\n",
			},
			want: &Response{
				DataType: string(Array),
				Size:     120,
			},
		},
		{
			name: "Array nested",
			inputs: []string{
				"*2\r\n*2\r\n*2\r\n:1\r\n:2\r\n*2\r\n:3\r\n:4\r\n",
				"*2\r\n*2\r\n$5\r\nhello\r\n$5\r\nwo",
				"rld\r\n*1\r\n-Error\r\n",
			},
			want: &Response{
				DataType: string(Array),
				Size:     19,
			},
		},
		{
			name: "Array nested",
			inputs: []string{
				"*2\r\n*2\r\n*2\r\n:1\r\n:2\r\n*2\r\n:3\r\n:4\r\n",
				"*2\r\n*2\r\n$5\r\nhel",
				"lo\r\n$5\r\nwo",
				"rld\r\n*1\r\n-Error\r\n",
			},
			want: &Response{
				DataType: string(Array),
				Size:     19,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0)
			var objs []*role.Object
			var err error
			for _, input := range tt.inputs {
				objs, err = d.Decode(zerocopy.NewBuffer([]byte(input)), t0)
			}
			assert.NoError(t, err)

			obj := objs[0].Obj.(*Response)
			assert.Equal(t, tt.want.DataType, obj.DataType)
			assert.Equal(t, tt.want.Size, obj.Size)
		})
	}
}
