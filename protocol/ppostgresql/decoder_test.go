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

package ppostgresql

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/splitio"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol/role"
)

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name    string
		inputs  [][]byte
		request *Request
	}{
		{
			name: "SimpleQuery",
			inputs: [][]byte{
				{
					'Q',
					0x00, 0x00, 0x00, 0x0C,
					'S', 'E', 'L', 'E', 'C', 'T', ' ', '1', 0x00,
				},
			},
			request: &Request{
				Packet: &QueryPacket{
					Statement: "SELECT 1",
				},
				Size: 13,
			},
		},
		{
			name: "EmptyQuery",
			inputs: [][]byte{
				{
					'Q',
					0x00, 0x00, 0x00, 0x05,
					0x00,
				},
			},
			request: &Request{
				Packet: &QueryPacket{
					Statement: "",
				},
				Size: 6,
			},
		},
		{
			name: "QueryWithComments",
			inputs: [][]byte{
				{
					'Q',
					0x00, 0x00, 0x00, 0x1D,
					'S', 'E', 'L', 'E', 'C', 'T', ' ', '1', ' ',
					'-', '-', ' ', 't', 'e', 's', 't', ' ', 'c', 'o', 'm', 'm', 'e', 'n', 't', 0x00,
				},
			},
			request: &Request{
				Packet: &QueryPacket{
					Statement: "SELECT 1 -- test comment",
				},
				Size: 30,
			},
		},
		{
			name: "ParameterizedQuery",
			inputs: [][]byte{
				{
					'P',
					0x00, 0x00, 0x00, 0x17,
					's', 't', 'm', 't', '_', '1', 0x00,
					'S', 'E', 'L', 'E', 'C', 'T', ' ', '$', '1', 0x00,
					0x00, 0x01,
				},
				{
					'B',
					0x00, 0x00, 0x00, 0x1A,
					0x00,
					's', 't', 'm', 't', '_', '1', 0x00,
					0x00, 0x00, 0x00, 0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					'1', '0', '0', '0',
				},
			},
			request: &Request{
				Packet: &QueryPacket{
					Statement: "SELECT $1",
				},
				Size: 51,
			},
		},
		{
			name: "LargeQuery",
			inputs: func() [][]byte {
				b := [][]byte{
					{
						'Q',
						0x00, 0x01, 0x00, 0x00,
						'S', 'E', 'L', 'E', 'C', 'T', ' ', '*', ' ', 'F', 'R', 'O', 'M', ' ',
					},
				}
				b = append(b, splitio.SplitChunk(bytes.Repeat([]byte{'a'}, 65517), common.ReadWriteBlockSize)...)
				b = append(b, []byte{0x00})
				return b
			}(),
			request: &Request{
				Packet: &QueryPacket{
					Statement: "SELECT * FROM " + string(bytes.Repeat([]byte{'a'}, maxStatementSize-14)),
				},
				Size: 65537,
			},
		},
		{
			name: "MultipleStatements",
			inputs: [][]byte{
				{
					'Q',
					0x00, 0x00, 0x00, 0x16,
					'S', 'E', 'L', 'E', 'C', 'T', ' ', '1', ';',
					'S', 'E', 'L', 'E', 'C', 'T', ' ', '2', 0x00,
				},
			},
			request: &Request{
				Packet: &QueryPacket{
					Statement: "SELECT 1;SELECT 2",
				},
				Size: 23,
			},
		},
		{
			name: "QueryWithEscapes",
			inputs: [][]byte{
				{
					'Q',
					0x00, 0x00, 0x00, 0x13,
					'S', 'E', 'L', 'E', 'C', 'T', ' ', '\'', '\\', 'n', '\\', 't', '\\', '\'', '\'', 0x00,
				},
			},
			request: &Request{
				Packet: &QueryPacket{
					Statement: "SELECT '\\n\\t\\''",
				},
				Size: 20,
			},
		},
		{
			name: "QueryWithBinaryData",
			inputs: [][]byte{
				{
					'Q',
					0x00, 0x00, 0x00, 0x12,
					'S', 'E', 'L', 'E', 'C', 'T', ' ', '\\', 'x', '0', '1', '2', '3', 0x00,
				},
			},
			request: &Request{
				Packet: &QueryPacket{
					Statement: "SELECT \\x0123",
				},
				Size: 19,
			},
		},
		{
			name: "DDLStatement",
			inputs: [][]byte{
				{
					'Q',
					0x00, 0x00, 0x00, 0x1E,
					'C', 'R', 'E', 'A', 'T', 'E', ' ', 'T', 'A', 'B', 'L', 'E', ' ',
					't', 'e', 's', 't', '(', 'i', 'd', ' ', 'i', 'n', 't', ')', 0x00,
				},
			},
			request: &Request{
				Packet: &QueryPacket{
					Statement: "CREATE TABLE test(id int)",
				},
				Size: 31,
			},
		},
		{
			name: "DescribePreparedStatement",
			inputs: [][]byte{
				{
					'D',
					0x00, 0x00, 0x00, 0x0C,
					'S',
					's', 't', 'm', 't', '_', '1', 0x00,
				},
			},
			request: &Request{
				Packet: &DescribePacket{
					Type:   "S",
					Object: "stmt_1",
				},
				Size: 13,
			},
		},
		{
			name: "DescribePortal",
			inputs: [][]byte{
				{
					'D',
					0x00, 0x00, 0x00, 0x0E,
					'P',
					'p', 'o', 'r', 't', 'a', 'l', '_', '1', 0x00,
				},
			},
			request: &Request{
				Packet: &DescribePacket{
					Type:   "P",
					Object: "portal_1",
				},
				Size: 15,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0)
			var err error
			var objs []*role.Object
			for _, input := range tt.inputs {
				objs, err = d.Decode(zerocopy.NewBuffer(input), t0)
			}
			assert.NoError(t, err)

			obj := objs[0].Obj.(*Request)
			assert.Equal(t, tt.request.Size, obj.Size)
			assert.Equal(t, tt.request.Packet, obj.Packet)
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name     string
		inputs   [][]byte
		response *Response
	}{
		{
			name: "SimpleQueryResponse",
			inputs: [][]byte{
				{
					'T',
					0x00, 0x00, 0x00, 0x32,
					0x00, 0x02,
					'i', 'd', 0x00,
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x17,
					0x00, 0x00, 0x00, 0x04,
					0xFF, 0xFF, 0xFF, 0xFF,
					0x00, 0x00,
					'n', 'a', 'm', 'e', 0x00,
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x19,
					0x00, 0x00, 0x00, 0x40,
					0xFF, 0xFF, 0xFF, 0xFF,
					0x00, 0x00,
				},
				{
					'D',
					0x00, 0x00, 0x00, 0x18,
					0x00, 0x02,
					0x00, 0x00, 0x00, 0x04,
					'1', '0', '0', '0',
					0x00, 0x00, 0x00, 0x06,
					't', 'e', 's', 't', '1', '2',
				},
				{
					'C',
					0x00, 0x00, 0x00, 0x0D,
					'S', 'E', 'L', 'E', 'C', 'T', ' ', '1', 0x00,
				},
				{
					'Z',
					0x00, 0x00, 0x00, 0x05,
					'I',
				},
			},
			response: &Response{
				Packet: &CommandCompletePacket{
					Command: "SELECT",
					Rows:    1,
				},
				Size: 90,
			},
		},
		{
			name: "ErrorResponse",
			inputs: [][]byte{
				{
					'E',
					0x00, 0x00, 0x00, 0x39,
					'S', 'E', 'R', 'R', 'O', 'R', 0x00,
					'C', '2', '8', 'P', '0', '1', 0x00,
					'M', 'd', 'u', 'p', 'l', 'i', 'c', 'a', 't', 'e', ' ', 'k', 'e', 'y', ' ', 'v', 'a', 'l', 'u', 'e', 0x00,
					'P', '1', '5', 0x00,
					'F', 'p', 'o', 's', 't', 'g', 'r', 'e', 's', '.', 'c', 0x00,
					'L', '1', '2', '3', '4', 0x00,
					'R', 'e', 'x', 'e', 'c', '_', 's', 'i', 'm', 'p', 'l', 'e', '_', 'q', 'u', 'e', 'r', 'y', 0x00,
					0x00,
				},
			},
			response: &Response{
				Packet: &ErrorPacket{
					Severity:     "ERROR",
					SQLStateCode: "28P01",
					Message:      "duplicate key value",
				},
				Size: 58,
			},
		},
		{
			name: "DescribeResponse",
			inputs: [][]byte{
				{
					'3',
					0x00, 0x00, 0x00, 0x25,
					'S',
					's', 't', 'm', 't', '1', 0x00,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x17,
					0x00, 0x01,
					'i', 'd', 0x00,
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x17,
					0x00, 0x04,
					0xFF, 0xFF, 0xFF, 0xFF,
					0x00, 0x00,
				},
			},
			response: &Response{
				Packet: &FlagPacket{
					Flag: "CloseCompleteOrDescribeResponse",
				},
				Size: 38,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 5432)
			var err error
			var objs []*role.Object
			for _, input := range tt.inputs {
				objs, err = d.Decode(zerocopy.NewBuffer(input), t0)
				if len(objs) > 0 {
					break
				}
			}
			assert.NoError(t, err)

			obj := objs[0].Obj.(*Response)
			assert.Equal(t, tt.response.Size, obj.Size)
			assert.Equal(t, tt.response.Packet, obj.Packet)
		})
	}
}

func TestDecodeFailed(t *testing.T) {
	tests := []struct {
		name   string
		inputs [][]byte
	}{
		{
			name: "InvalidMessageType",
			inputs: [][]byte{
				{'X', 0x00, 0x00, 0x00, 0x04},
			},
		},
		{
			name: "IncompleteLengthHeader",
			inputs: [][]byte{
				{'Q', 0x00, 0x00},
			},
		},
		{
			name: "NegativeMessageLength",
			inputs: [][]byte{
				{'Q', 0xFF, 0xFF, 0xFF, 0xFF},
			},
		},
		{
			name: "ExceedMaxMessageSize",
			inputs: [][]byte{
				{'Q', 0x00, 0x10, 0x00, 0x00},
			},
		},
		{
			name: "EmptyMessage",
			inputs: [][]byte{
				{},
			},
		},
		{
			name: "IncompleteParameterData",
			inputs: [][]byte{
				{'B', 0x00, 0x00, 0x00, 0x10, 0x00},
			},
		},
		{
			name: "InvalidParameterFormat",
			inputs: [][]byte{
				{'B', 0x00, 0x00, 0x00, 0x0C, 0x02},
			},
		},
		{
			name: "InvalidRowDescription",
			inputs: [][]byte{
				{'T', 0x00, 0x00, 0x00, 0x08, 0x00, 0x01},
			},
		},
		{
			name: "TruncatedErrorResponse",
			inputs: [][]byte{
				{'E', 0x00, 0x00, 0x00, 0x10, 'S'},
			},
		},
		{
			name: "InvalidDescribeResponse",
			inputs: [][]byte{
				{'3', 0x00, 0x00, 0x00, 0x06, 'X'},
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 3306)
			var err error
			var objs []*role.Object
			for _, input := range tt.inputs {
				objs, err = d.Decode(zerocopy.NewBuffer(input), t0)
			}
			assert.NoError(t, err)
			assert.Nil(t, objs)
		})
	}
}
