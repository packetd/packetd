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

package pamqp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol/role"
)

func TestChannelDecoderRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   [][]byte
		request *Request
	}{
		{
			name: "Channel.Open",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x14, 0x00, 0x0A,
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "Method",
				Size:      12,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Channel",
					Method: "Open",
				},
			},
		},
		{
			name: "Basic.Publish with exchange",
			input: [][]byte{
				{
					0x01, // Method Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x14,
					0x00, 0x3C, 0x00, 0x28,
					0x00, 0x05, 'a', 'm', 'q', 'p', '.', 'd', 'i', 'r',
					0x00, 0x03, 'k', 'e', 'y',
					0x00,
					0xCE,
				},
				{
					0x02, // ContentHeader Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0D,
					0x00, 0x3C, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0B,
					0x02,
					0xCE,
				},
				{
					0x03, // ContentBody Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0B,
					'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd',
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "ContentBody",
				Size:      68,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Publish",
				},
			},
		},
		{
			name: "Channel.Close with error",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x13,
					0x00, 0x14, 0x00, 0x28,
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x05, 'e', 'r', 'r', 'o', 'r',
					0x00, 0x00, 0x00, 0x00,
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "Method",
				Size:      27,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Channel",
					Method: "Close",
				},
			},
		},
		{
			name: "Multi-frame content message",
			input: [][]byte{
				{
					0x02, // ContentHeader Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0C,
					0x00, 0x3C, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0B,
					0xCE,
				},
				{
					0x03, // ContentBody Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0B,
					'h', 'e', 'l', 'l', 'o', ' ', 'w', 'o', 'r', 'l', 'd',
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "ContentBody",
				Size:      39,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "",
				},
			},
		},
		{
			name: "Heartbeat frame",
			input: [][]byte{
				{
					0x08,
					0x00, 0x00,
					0x00, 0x00, 0x00, 0x00,
					0xCE,
				},
			},
			request: &Request{
				ChannelID:   0,
				FrameType:   "Heartbeat",
				Size:        8,
				Proto:       "AMQP",
				ClassMethod: &NamedClassMethod{},
			},
		},
		{
			name: "Queue.Declare with arguments",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x14,
					0x00, 0x32, 0x00, 0x0A,
					0x00, 0x00, 0x05, 'q', 'u', 'e', 'u', 'e',
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00,
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "Method",
				Size:      28,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Queue",
					Method: "Declare",
				},
				Packet: &Packet{
					QueueName: "queue",
				},
			},
		},
		{
			name: "Basic.Get with empty queue",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x08,
					0x00, 0x3C, 0x00, 0x46,
					0x00, 0x00,
					0x00, 0x00,
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "Method",
				Size:      16,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Get",
				},
				Packet: &Packet{},
			},
		},
		{
			name: "Connection.Start",
			input: [][]byte{
				{
					0x01,
					0x00, 0x00,
					0x00, 0x00, 0x00, 0x0C,
					0x00, 0x0A, 0x00, 0x0A,
					0x00, 0x00,
					0x00, 0x09,
					0x00, 0x00, 0x00, 0x00,
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 0,
				FrameType: "Method",
				Size:      20,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Connection",
					Method: "Start",
				},
			},
		},
		{
			name: "Basic.Ack with delivery tag",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0D,
					0x00, 0x3C, 0x00, 0x50,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
					0x00,
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "Method",
				Size:      21,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Ack",
				},
			},
		},
		{
			name: "Channel.Flow control",
			input: [][]byte{
				{
					0x01, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x05,
					0x00, 0x14, 0x00, 0x14,
					0x01,
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "Method",
				Size:      13,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Channel",
					Method: "Flow",
				},
			},
		},
		{
			name: "Basic.Deliver message",
			input: [][]byte{
				{
					0x01, // Method Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x23,
					0x00, 0x3C, 0x00, 0x3C,
					0x07, 'd', 'e', 'f', 'a', 'u', 'l', 't',
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
					0x00,
					0x05, 't', 'e', 's', 't', '1',
					0x07, 'r', 'o', 'u', 't', 'i', 'n', 'g',
					0xCE,
				},
				{
					0x02, // ContentHeader Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0D,
					0x00, 0x3C, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0C,
					0x02,
					0xCE,
				},
				{
					0x03, // ContentBody Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0C,
					't', 'e', 's', 't', ' ', 'm', 'e', 's', 's', 'a', 'g', 'e',
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "ContentBody",
				Size:      84,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Deliver",
				},
				Packet: &Packet{
					ExchangeName: "test1",
					RoutingKey:   "routing",
				},
			},
		},
		{
			name: "Basic.Consume queue",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x19,
					0x00, 0x3C, 0x00, 0x14,
					0x00, 0x00,
					0x05, 'm', 'y', '_', 'q', '1',
					0x07, 'm', 'y', '_', 'c', 'o', 'n', 's',
					0x00, 0x00, 0x00, 0x00,
					0x01,
					0xCE,
				},
			},
			request: &Request{
				ChannelID: 1,
				FrameType: "Method",
				Size:      33,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Consume",
				},
				Packet: &Packet{
					QueueName: "my_q1",
				},
			},
		},
	}

	var st socket.TupleRaw
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := newChannelDecoder(1, st, 0)
			defer cd.Free()

			var got *role.Object
			var err error
			for _, chunk := range tt.input {
				got, err = cd.Decode(chunk, t0)
			}

			assert.NoError(t, err)
			obj := got.Obj.(*Request)

			assert.Equal(t, tt.request.Size, obj.Size)
			assert.Equal(t, tt.request.Proto, obj.Proto)
			assert.Equal(t, tt.request.FrameType, obj.FrameType)
			assert.Equal(t, tt.request.ClassMethod, obj.ClassMethod)
			assert.Equal(t, tt.request.Packet, obj.Packet)
		})
	}
}

func TestChannelDecoderResponse(t *testing.T) {
	tests := []struct {
		name        string
		input       [][]byte
		trailerKeys []string
		response    *Response
	}{
		{
			name: "Connection.Start-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x00,
					0x00, 0x00, 0x00, 0x19,
					0x00, 0x0A, 0x00, 0x0B,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x04, 'A', 'M', 'Q', 'P',
					0x00, 0x00, 0x00, 0x00,
					0x05, 'e', 'n', '_', 'U', 'S',
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 0,
				FrameType: "Method",
				Size:      33,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Connection",
					Method: "Start-Ok",
				},
			},
		},
		{
			name: "Channel.Open-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x14, 0x00, 0x0B,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      12,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Channel",
					Method: "Open-Ok",
				},
			},
		},
		{
			name: "Basic.Consume-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0B,
					0x00, 0x3C, 0x00, 0x15,
					0x06, 'c', 'o', 'n', 's', '_', '1',
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      19,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Consume-Ok",
				},
			},
		},
		{
			name: "Basic.Get-Ok",
			input: [][]byte{
				{
					0x01, // Method Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x1D,
					0x00, 0x3C, 0x00, 0x47,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
					0x00,
					0x05, 't', 'e', 's', 't', '1',
					0x05, 'r', 'o', 'u', 't', 'e',
					0x00, 0x00, 0x00, 0x01,
					0xCE,
				},
				{
					0x02, // ContentHeader Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0D,
					0x00, 0x3C, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0A,
					0x02,
					0xCE,
				},
				{
					0x03, // ContentBody Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0A,
					't', 'e', 's', 't', ' ', 'm', 's', 'g', ' ', '1',
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "ContentBody",
				Size:      76,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Get-Ok",
				},
				Packet: &Packet{
					QueueName: "test1",
				},
			},
		},
		{
			name: "Basic.Get-Empty",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x06,
					0x00, 0x3C, 0x00, 0x48,
					0x00, 0x00,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      14,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Get-Empty",
				},
			},
		},
		{
			name: "Channel.Close-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x14, 0x00, 0x29,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      12,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Channel",
					Method: "Close-Ok",
				},
			},
		},
		{
			name: "Queue.Declare-Ok with message count",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x12,
					0x00, 0x32, 0x00, 0x0B,
					0x05, 'm', 'y', '_', 'q', '1',
					0x00, 0x00, 0x00, 0x64,
					0x00, 0x00, 0x00, 0x03,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      26,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Queue",
					Method: "Declare-Ok",
				},
				Packet: &Packet{
					QueueName: "my_q1",
				},
			},
		},
		{
			name: "Basic.Qos-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x3C, 0x00, 0x0B,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      12,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Qos-Ok",
				},
			},
		},
		{
			name: "Basic.Recover-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x3C, 0x00, 0x65,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      12,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Recover-Ok",
				},
			},
		},
		{
			name: "Exchange.Declare-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x28, 0x00, 0x0B,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      12,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Exchange",
					Method: "Declare-Ok",
				},
			},
		},
		{
			name: "Tx.Commit-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x5A, 0x00, 0x15,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      12,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Tx",
					Method: "Commit-Ok",
				},
			},
		},
		{
			name: "Tx.Rollback-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x5A, 0x00, 0x1F,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      12,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Tx",
					Method: "Rollback-Ok",
				},
			},
		},
		{
			name: "Connection.Tune-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x00,
					0x00, 0x00, 0x00, 0x10,
					0x00, 0x0A, 0x00, 0x1F,
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 0,
				FrameType: "Method",
				Size:      24,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Connection",
					Method: "Tune-Ok",
				},
			},
		},
		{
			name: "Connection.Secure-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x00,
					0x00, 0x00, 0x00, 0x08,
					0x00, 0x0A, 0x00, 0x15,
					0x00, 0x00, 0x00, 0x00,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 0,
				FrameType: "Method",
				Size:      16,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Connection",
					Method: "Secure-Ok",
				},
			},
		},
		{
			name: "Basic.Return with message",
			input: [][]byte{
				{
					0x01, // Method Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x1D,
					0x00, 0x3C, 0x00, 0x32,
					0x00, 0x96,
					0x08, 'n', 'o', ' ', 'r', 'o', 'u', 't', 'e',
					0x05, 't', 'e', 's', 't', '1',
					0x07, 'r', 'o', 'u', 't', 'i', 'n', 'g',
					0xCE,
				},
				{
					0x02, // ContentHeader Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0D,
					0x00, 0x3C, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0C,
					0x02,
					0xCE,
				},
				{
					0x03, // ContentBody Frame
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0C,
					'r', 'e', 't', 'u', 'r', 'n', ' ', 'm', 's', 'g', ' ', '1',
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "ContentBody",
				Size:      78,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Basic",
					Method: "Return",
				},
				Packet: &Packet{
					ExchangeName: "test1",
					RoutingKey:   "routing",
				},
			},
		},
		{
			name: "Connection.Close with error",
			input: [][]byte{
				{
					0x01,
					0x00, 0x00,
					0x00, 0x00, 0x00, 0x13,
					0x00, 0x0A, 0x00, 0x32,
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x05, 'e', 'r', 'r', 'o', 'r',
					0x00, 0x00, 0x00, 0x00,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 0,
				FrameType: "Method",
				Size:      27,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Connection",
					Method: "Close",
				},
			},
		},
		{
			name: "Channel.Flow-Ok",
			input: [][]byte{
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x05,
					0x00, 0x14, 0x00, 0x15,
					0x01,
					0xCE,
				},
			},
			response: &Response{
				ChannelID: 1,
				FrameType: "Method",
				Size:      13,
				Proto:     "AMQP",
				ClassMethod: &NamedClassMethod{
					Class:  "Channel",
					Method: "Flow-Ok",
				},
			},
		},
	}

	var st socket.TupleRaw
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := newChannelDecoder(2, st, 8080)
			defer cd.Free()

			var got *role.Object
			var err error
			for _, chunk := range tt.input {
				got, err = cd.Decode(chunk, t0)
			}

			assert.NoError(t, err)
			obj := got.Obj.(*Response)

			assert.Equal(t, tt.response.Size, obj.Size)
			assert.Equal(t, tt.response.Proto, obj.Proto)
			assert.Equal(t, tt.response.ClassMethod, obj.ClassMethod)
			assert.Equal(t, tt.response.Packet, obj.Packet)
		})
	}
}

func TestChannelDecoderFailed(t *testing.T) {
	tests := []struct {
		name  string
		input [][]byte
	}{
		{
			name: "Invalid frame type",
			input: [][]byte{
				{0x09},
			},
		},
		{
			name: "Incomplete method frame",
			input: [][]byte{
				{0x01, 0x00, 0x01},
			},
		},
		{
			name: "Invalid class method",
			input: [][]byte{
				{
					0x01, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0xFF, 0xFF, 0x00, 0x00,
					0xCE,
				},
			},
		},
		{
			name: "Frame size exceeds limit",
			input: [][]byte{
				{
					0x01, 0x00, 0x01,
					0xFF, 0xFF, 0xFF, 0xFF,
					0x00, 0x3C, 0x00, 0x14,
					0xCE,
				},
			},
		},
		{
			name: "Invalid content header frame",
			input: [][]byte{
				{
					0x02, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x3C, 0x00, 0x00,
				},
			},
		},
		{
			name: "Missing end marker",
			input: [][]byte{
				{
					0x01, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x3C, 0x00, 0x14,
				},
			},
		},
		{
			name: "Invalid channel ID",
			input: [][]byte{
				{
					0x01, 0xFF, 0xFF,
					0x00, 0x00, 0x00, 0x04,
					0x00, 0x3C, 0x00, 0x14,
					0xCE,
				},
			},
		},
		{
			name: "Invalid heartbeat frame",
			input: [][]byte{
				{
					0x08, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x01,
					0xCE,
				},
			},
		},
		{
			name: "Invalid string length",
			input: [][]byte{
				{
					0x01, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x08,
					0x00, 0x3C, 0x00, 0x14,
					0xFF,
					't', 'e', 's', 't',
					0xCE,
				},
			},
		},
		{
			name: "Frame size mismatch",
			input: [][]byte{
				{
					0x01, 0x00, 0x01,
					0x00, 0x00, 0x00, 0x08,
					0x00, 0x3C, 0x00, 0x14,
					0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0xCE,
				},
			},
		},
	}

	var st socket.TupleRaw
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := newChannelDecoder(1, st, 0)
			defer cd.Free()

			var objs *role.Object
			for _, chunk := range tt.input {
				objs, _ = cd.Decode(chunk, t0)
			}
			assert.Nil(t, objs)
		})
	}
}

func TestDecodeShortString(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		s      string
		length int
		err    error
	}{
		{
			name:   "Normal short string",
			input:  []byte{0x05, 'h', 'e', 'l', 'l', 'o'},
			s:      "hello",
			length: 6,
		},
		{
			name:   "Empty string",
			input:  []byte{0x00},
			s:      "",
			length: 1,
		},
		{
			name:  "Empty input",
			input: []byte{},
			err:   errDecodeString,
		},
		{
			name:  "Insufficient length",
			input: []byte{0x05, 'h', 'e'},
			err:   errDecodeString,
		},
		{
			name:  "Max length string",
			input: append([]byte{255}, make([]byte, 255)...),
			err:   errDecodeString,
		},
		{
			name:   "UTF8 string",
			input:  append([]byte{0x0C}, []byte("中文测试")...),
			s:      "中文测试",
			length: 13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, length, err := decodeShortString(tt.input)
			assert.Equal(t, tt.s, s)
			assert.Equal(t, tt.length, length)
			assert.Equal(t, tt.err, err)
		})
	}
}
