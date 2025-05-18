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
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/splitio"
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
			name: "MultipleStreams1",
			input: [][]byte{
				// StreamID = 1
				buildFrame(1, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "POST",
						":path":   "/api1",
					}),
				),
				// StreamID = 3
				buildFrame(3, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "POST",
						":path":   "/api2",
					}),
				),
				// StreamID = 1 data
				buildFrame(1, frameData, 0, []byte("part1")),
				// StreamID = 2 end
				buildFrame(3, frameData, flagEndStream, []byte("part2")),
				// StreamID = 1 end
				buildFrame(1, frameData, flagEndStream, []byte("part3")),
			},
			objs: []*role.Object{
				role.NewRequestObject(
					&Request{
						StreamID: 3,
						Method:   "POST",
						Path:     "/api2",
						Proto:    "HTTP/2",
						Size:     50,
						Header:   http.Header{},
					}),
				role.NewRequestObject(&Request{
					StreamID: 1,
					Method:   "POST",
					Proto:    "HTTP/2",
					Path:     "/api1",
					Size:     64,
					Header:   http.Header{},
				}),
			},
		},
		{
			name: "StreamWithPriority",
			input: [][]byte{
				buildFrame(1, framePriority, 0, []byte{
					0x00, 0x00, 0x00, 0x00,
					0xFF, // Weight
				}),
				buildFrame(1, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "PUT",
						":path":   "/upload",
					}),
				),
				buildFrame(3, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/status",
					}),
				),
				buildFrame(1, frameData, flagEndStream, []byte("data")),
				buildFrame(3, frameData, flagEndStream, []byte("status")),
			},
			objs: []*role.Object{
				role.NewRequestObject(&Request{
					StreamID: 1,
					Method:   "PUT",
					Path:     "/upload",
					Proto:    "HTTP/2",
					Size:     64,
					Header:   http.Header{},
				}),
				role.NewRequestObject(&Request{
					StreamID: 3,
					Method:   "GET",
					Path:     "/status",
					Proto:    "HTTP/2",
					Size:     52,
					Header:   http.Header{},
				}),
			},
		},
		{
			name: "InterruptedStream",
			input: [][]byte{
				buildFrame(1, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "POST",
						":path":   "/create",
					}),
				),
				buildFrame(3, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/info",
					}),
				),
				buildFrame(1, frameData, 0, []byte("partial")),
				buildFrame(3, frameData, flagEndStream, []byte("complete")),
				// Stream 1 未结束
			},
			objs: []*role.Object{
				role.NewRequestObject(&Request{
					StreamID: 3,
					Method:   "GET",
					Path:     "/info",
					Proto:    "HTTP/2",
					Size:     52,
					Header:   http.Header{},
				}),
			},
		},
		{
			name: "WindowUpdateInterleaving",
			input: [][]byte{
				buildFrame(1, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/large",
					}),
				),
				buildFrame(0, frameWindowUpdate, 0, []byte{0x00, 0x01, 0x00, 0x00}), // 全局窗口更新
				buildFrame(1, frameWindowUpdate, 0, []byte{0x00, 0x00, 0x40, 0x00}), // 流窗口更新
				buildFrame(1, frameData, flagEndStream, bytes.Repeat([]byte("a"), 1024)),
			},
			objs: []*role.Object{
				role.NewRequestObject(&Request{
					StreamID: 1,
					Method:   "GET",
					Path:     "/large",
					Proto:    "HTTP/2",
					Size:     1082,
					Header:   http.Header{},
				}),
			},
		},
		{
			name: "StreamDependencyChain",
			input: [][]byte{
				buildFrame(1, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/parent",
					}),
				),
				buildFrame(3, framePriority, 0, []byte{
					0x80, 0x00, 0x00, 0x01, // 依赖 Stream1
					0xFF,
				}),
				buildFrame(1, frameData, flagEndStream, []byte("complete")),
				buildFrame(3, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/child",
					}),
				),
				buildFrame(5, framePriority, 0, []byte{
					0x80, 0x00, 0x00, 0x03, // 依赖 Stream2
					0xFF,
				}),
				buildFrame(5, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/grandchild",
					}),
				),
				buildFrame(5, frameData, flagEndStream, []byte("complete")),
				buildFrame(3, frameData, flagEndStream, []byte("complete")),
			},
			objs: []*role.Object{
				role.NewRequestObject(&Request{
					StreamID: 1,
					Method:   "GET",
					Path:     "/parent",
					Proto:    "HTTP/2",
					Size:     54,
					Header:   http.Header{},
				}),
				role.NewRequestObject(&Request{
					StreamID: 5,
					Method:   "GET",
					Path:     "/grandchild",
					Proto:    "HTTP/2",
					Size:     72,
					Header:   http.Header{},
				}),
				role.NewRequestObject(&Request{
					StreamID: 3,
					Method:   "GET",
					Path:     "/child",
					Proto:    "HTTP/2",
					Size:     67,
					Header:   http.Header{},
				}),
			},
		},
		{
			name: "RstStreamHandling",
			input: [][]byte{
				buildFrame(1, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "DELETE",
						":path":   "/resource",
					}),
				),
				buildFrame(3, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "POST",
						":path":   "/create",
					}),
				),
				buildFrame(1, frameRSTStream, 0, []byte{0x00, 0x00, 0x00, 0x08}), // CANCEL
				buildFrame(3, frameData, flagEndStream, []byte("created")),
			},
			objs: []*role.Object{
				role.NewRequestObject(&Request{
					StreamID: 3,
					Method:   "POST",
					Path:     "/create",
					Proto:    "HTTP/2",
					Size:     54,
					Header:   http.Header{},
				}),
			},
		},
		{
			name: "StreamIDOrderViolation",
			input: [][]byte{
				buildFrame(3, frameHeaders, flagEndHeaders, buildHeadersFramePayload(false, 0, nil)),
				buildFrame(1, frameHeaders, flagEndHeaders, buildHeadersFramePayload(false, 0, nil)),
			},
			objs: []*role.Object{}, // 无 data 不成 *Object
		},
		{
			name: "CutStream",
			input: func() [][]byte {
				b := buildFrame(1, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/index.html",
					}),
				)
				b = append(b, buildFrame(1, frameData, flagEndStream, bytes.Repeat([]byte("x"), 100))...)
				return [][]byte{b[:5], b[5:]}
			}(),
			objs: []*role.Object{
				role.NewRequestObject(&Request{
					StreamID: 1,
					Method:   "GET",
					Path:     "/index.html",
					Proto:    "HTTP/2",
					Size:     150,
					Header:   http.Header{},
				}),
			},
		},
		{
			name: "MultipleDataFrames",
			input: [][]byte{
				buildFrame(1, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/index.html",
					}),
				),
				buildFrame(1, frameData, 0, bytes.Repeat([]byte("a"), 100)),
				buildFrame(1, frameData, 0, bytes.Repeat([]byte("b"), 100)),
				splitio.SplitChunk(buildFrame(1, frameData, 0, bytes.Repeat([]byte("c"), common.ReadWriteBlockSize)), common.ReadWriteBlockSize)[0],
				splitio.SplitChunk(buildFrame(1, frameData, 0, bytes.Repeat([]byte("c"), common.ReadWriteBlockSize)), common.ReadWriteBlockSize)[1],
				buildFrame(1, frameData, flagEndStream, bytes.Repeat([]byte("d"), 100)),
			},
			objs: []*role.Object{
				role.NewRequestObject(&Request{
					StreamID: 1,
					Method:   "GET",
					Path:     "/index.html",
					Proto:    "HTTP/2",
					Size:     4473,
					Header:   http.Header{},
				}),
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
