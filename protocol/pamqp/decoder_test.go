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
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol/role"
)

func TestDecoderDecode(t *testing.T) {
	tests := []struct {
		name  string
		input [][]byte
		objs  []*role.Object
	}{
		{
			name: "Multiple Channels",
			input: [][]byte{
				// Channel1 Connection.Start
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x0E,
					0x00, 0x0A, 0x00, 0x0A,
					0x00, 0x00, 0x00, 0x00,
					0x05, 'P', 'L', 'A', 'I', 'N',
					0xCE,
				},
				// Channel2 Connection.Start
				{
					0x01,
					0x00, 0x02,
					0x00, 0x00, 0x00, 0x0E,
					0x00, 0x0A, 0x00, 0x0A,
					0x00, 0x00, 0x00, 0x00,
					0x05, 'P', 'L', 'A', 'I', 'N',
					0xCE,
				},
				// Channel1 Channel.Open
				{
					0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x06,
					0x00, 0x14, 0x00, 0x0A, 0x00, 0x00,
					0xCE,
				},
				// Channel2 Channel.Open
				{
					0x01,
					0x00, 0x02,
					0x00, 0x00, 0x00, 0x06,
					0x00, 0x14, 0x00, 0x0A, 0x00, 0x00,
					0xCE,
				},
			},
			objs: []*role.Object{
				{
					Role: role.Request,
					Obj: &Request{
						ChannelID: 1,
						FrameType: "Method",
						Size:      22,
						Proto:     "AMQP",
						ClassMethod: &NamedClassMethod{
							Class:  "Connection",
							Method: "Start",
						},
						ErrCode: "OK",
					},
				},
				{
					Role: role.Request,
					Obj: &Request{
						ChannelID: 2,
						FrameType: "Method",
						Size:      22,
						Proto:     "AMQP",
						ClassMethod: &NamedClassMethod{
							Class:  "Connection",
							Method: "Start",
						},
						ErrCode: "OK",
					},
				},
				{
					Role: role.Request,
					Obj: &Request{
						ChannelID: 1,
						FrameType: "Method",
						Size:      14,
						Proto:     "AMQP",
						ClassMethod: &NamedClassMethod{
							Class:  "Channel",
							Method: "Open",
						},
						ErrCode: "OK",
					},
				},
				{
					Role: role.Request,
					Obj: &Request{
						ChannelID: 2,
						FrameType: "Method",
						Size:      14,
						Proto:     "AMQP",
						ClassMethod: &NamedClassMethod{
							Class:  "Channel",
							Method: "Open",
						},
						ErrCode: "OK",
					},
				},
			},
		},
	}

	var st socket.Tuple
	t0 := time.Now()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := NewDecoder(st, 0)
			defer dec.Free()

			var lst []*role.Object
			for _, chunk := range tt.input {
				objs, err := dec.Decode(zerocopy.NewBuffer(chunk), t0)
				if err != nil {
					break
				}
				if objs != nil {
					lst = append(lst, objs...)
				}
			}

			assert.Equal(t, len(tt.objs), len(lst))
			for idx, obj := range lst {
				assert.Equal(t, tt.objs[idx].Role, obj.Role)
				switch obj.Role {
				case role.Request:
					req := obj.Obj.(*Request)
					req.Time = time.Time{}
					req.Host = ""
					assert.Equal(t, tt.objs[idx].Obj.(*Request), req)
				}
			}
		})
	}
}
