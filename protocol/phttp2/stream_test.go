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

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/protocol/role"
)

const (
	clientStreamID = 1
	serverStreamID = 2
)

func buildFrame(streamID int, frameType, flags uint8, payload []byte) []byte {
	length := len(payload)

	b := make([]byte, 9)
	b[0] = byte(length >> 16)
	b[1] = byte(length >> 8)
	b[2] = byte(length)
	b[3] = frameType
	b[4] = flags
	b[5] = byte(streamID >> 24)
	b[6] = byte(streamID >> 16)
	b[7] = byte(streamID >> 8)
	b[8] = byte(streamID)

	b = append(b, payload...)
	return b
}

func buildHeadersFramePayload(priority bool, padLen int, headers map[string]string) []byte {
	var buf bytes.Buffer

	if padLen > 0 {
		buf.WriteByte(byte(padLen))
	}

	// 处理优先级
	if priority {
		buf.Write([]byte{0x80, 0x00, 0x00, 0x01}) // StreamID = 1
		buf.WriteByte(0xFF)                       // Weight=255
	}

	writeHeader := func(name, value string) {
		buf.WriteByte(0x40) // 01000000: Literal Header Field with Incremental Indexing
		buf.WriteByte(byte(len(name)))
		buf.WriteString(name)
		buf.WriteByte(byte(len(value)))
		buf.WriteString(value)
	}

	// 按顺序处理伪头部
	for h := range pseudoHeaders {
		if val, ok := headers[h]; ok {
			writeHeader(h, val)
			delete(headers, h)
		}
	}

	for name, val := range headers {
		writeHeader(name, val)
	}

	if padLen > 0 {
		buf.Write(make([]byte, padLen))
	}
	return buf.Bytes()
}

func buildPushPromiseFramePayload(padLen int, headers map[string]string) []byte {
	var buf bytes.Buffer

	if padLen > 0 {
		buf.WriteByte(0x00)
		buf.WriteByte(byte(padLen))
	}

	buf.Write([]byte{0x00, 0x00, 0x00, 0x02}) // StreamID = 2
	writeHeader := func(name, value string) {
		buf.WriteByte(0x40) // 01000000: Literal Header Field with Incremental Indexing
		buf.WriteByte(byte(len(name)))
		buf.WriteString(name)
		buf.WriteByte(byte(len(value)))
		buf.WriteString(value)
	}

	// 按顺序处理伪头部
	for h := range pseudoHeaders {
		if val, ok := headers[h]; ok {
			writeHeader(h, val)
			delete(headers, h)
		}
	}

	for name, val := range headers {
		writeHeader(name, val)
	}

	if padLen > 0 {
		buf.Write(make([]byte, padLen))
	}
	return buf.Bytes()
}

func buildDataFramePayload(padLen int, data []byte) []byte {
	buf := make([]byte, 1+len(data)+padLen)
	buf[0] = byte(padLen)
	copy(buf[1:], data)
	return buf
}

func TestStreamDecoderRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   [][]byte
		request *Request
	}{
		{
			name: "DataPadded",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders|flagEndStream,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method":    "GET",
						":scheme":    "https",
						":path":      "/api/v1/users",
						":authority": "example.com",
						"user-agent": "test-client",
					}),
				),
				buildFrame(clientStreamID, frameData, flagEndStream|flagPadded,
					buildDataFramePayload(5, []byte("hello")),
				),
			},
			request: &Request{
				Proto:     "HTTP/2",
				Method:    "GET",
				Scheme:    "https",
				Path:      "/api/v1/users",
				Size:      126,
				Authority: "example.com",
				Header: http.Header{
					"User-Agent": []string{"test-client"},
				},
			},
		},
		{
			name: "Priority",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders|flagEndStream|flagPriority,
					buildHeadersFramePayload(true, 0, map[string]string{
						":method":    "POST",
						":scheme":    "http",
						":path":      "/submit",
						":authority": "test.local",
					}),
				),
				buildFrame(clientStreamID, frameData, flagEndStream|flagPadded,
					buildDataFramePayload(0, []byte("hello")),
				),
			},
			request: &Request{
				Proto:     "HTTP/2",
				Method:    "POST",
				Scheme:    "http",
				Path:      "/submit",
				Size:      95,
				Authority: "test.local",
				Header:    http.Header{},
			},
		},
		{
			name: "HeaderPadded",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders|flagPadded,
					buildHeadersFramePayload(false, 4, map[string]string{
						":method": "PUT",
						":path":   "/files/1",
					}),
				),
				buildFrame(clientStreamID, frameData, flagEndStream|flagPadded,
					buildDataFramePayload(8, []byte("file content")),
				),
			},
			request: &Request{
				Proto:  "HTTP/2",
				Method: "PUT",
				Path:   "/files/1",
				Size:   73,
				Header: http.Header{},
			},
		},
		{
			name: "ContinuationFrames",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, 0,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "HEAD",
					}),
				),
				buildFrame(clientStreamID, frameContinuation, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":path": "/status",
					}),
				),
				buildFrame(clientStreamID, frameData, flagEndStream,
					buildDataFramePayload(0, []byte("hello")),
				),
			},
			request: &Request{
				Proto:  "HTTP/2",
				Method: "HEAD",
				Path:   "/status",
				Size:   62,
				Header: http.Header{},
			},
		},
		{
			name: "MultipleDataFrames",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "POST",
						":path":   "/upload",
					}),
				),
				buildFrame(clientStreamID, frameData, 0, []byte("part1")),
				buildFrame(clientStreamID, frameData, 0, []byte("part2")),
				buildFrame(clientStreamID, frameData, 0, []byte("part3")),
				buildFrame(clientStreamID, frameData, flagEndStream, []byte("part4")),
			},
			request: &Request{
				Proto:  "HTTP/2",
				Method: "POST",
				Path:   "/upload",
				Size:   94,
				Header: http.Header{},
			},
		},
		{
			name: "PushPromiseFrame",
			input: [][]byte{
				buildFrame(clientStreamID, framePushPromise, flagEndHeaders,
					buildPushPromiseFramePayload(0, map[string]string{
						":method": "GET",
						":path":   "/style.css",
					}),
				),
				buildFrame(clientStreamID, frameData, flagEndStream, []byte("hello")),
			},
			request: &Request{
				Path:   "/style.css",
				Method: "GET",
				Proto:  "HTTP/2",
				Size:   58,
				Header: http.Header{},
			},
		},
		{
			name: "WindowUpdateFlowControl",
			input: [][]byte{
				buildFrame(clientStreamID, frameWindowUpdate, 0, []byte{0x00, 0x01, 0x00, 0x00}),
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "GET",
						":path":   "/flow",
					}),
				),
				buildFrame(clientStreamID, frameData, flagEndStream, []byte("hello")),
			},
			request: &Request{
				Method: "GET",
				Path:   "/flow",
				Proto:  "HTTP/2",
				Size:   62,
				Header: http.Header{},
			},
		},
		{
			name: "MaxPaddingSize",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders|flagPadded,
					buildHeadersFramePayload(false, 255, map[string]string{
						":method": "OPTIONS",
						":path":   "/max-pad",
					}),
				),
				buildFrame(clientStreamID, frameData, flagEndStream, []byte("hello")),
			},
			request: &Request{
				Method: "OPTIONS",
				Path:   "/max-pad",
				Proto:  "HTTP/2",
				Size:   312,
				Header: http.Header{},
			},
		},
		{
			name: "ZeroLengthData",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":method": "HEAD",
						":path":   "/empty",
					}),
				),
				buildFrame(clientStreamID, frameData, flagEndStream, []byte{}),
			},
			request: &Request{
				Method: "HEAD",
				Path:   "/empty",
				Proto:  "HTTP/2",
				Size:   46,
				Header: http.Header{},
			},
		},
	}

	var st socket.TupleRaw
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd := newStreamDecoder(1, st, 0, NewHeaderFieldDecoder())
			defer sd.Free()

			var got *role.Object
			var err error
			for _, chunk := range tt.input {
				got, err = sd.Decode(false, chunk, t0)
			}

			assert.NoError(t, err)
			obj := got.Obj.(*Request)

			assert.Equal(t, tt.request.Method, obj.Method)
			assert.Equal(t, tt.request.Path, obj.Path)
			assert.Equal(t, tt.request.Size, obj.Size)
			assert.Equal(t, tt.request.Authority, obj.Authority)
			assert.Equal(t, tt.request.Scheme, obj.Scheme)
			assert.Equal(t, tt.request.Header, obj.Header)
		})
	}
}

func TestStreamDecoderResponse(t *testing.T) {
	tests := []struct {
		name        string
		input       [][]byte
		trailerKeys []string
		response    *Response
	}{
		{
			name: "BasicResponse",
			input: [][]byte{
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":status": "200",
						"server":  "test-server",
					}),
				),
				buildFrame(serverStreamID, frameData, flagEndStream, []byte("OK")),
			},
			response: &Response{
				Proto:  "HTTP/2",
				Status: "200",
				Header: http.Header{
					"Server": []string{"test-server"},
				},
				Size: 53,
			},
		},
		{
			name: "ResponseWithData",
			input: [][]byte{
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":status": "404",
					}),
				),
				buildFrame(serverStreamID, frameData, flagEndStream, []byte("Not Found")),
			},
			response: &Response{
				Status: "404",
				Proto:  "HTTP/2",
				Size:   40,
				Header: http.Header{},
			},
		},
		{
			name: "ChunkedResponse",
			input: [][]byte{
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":status":        "206",
						"content-type":   "video/mp4",
						"content-length": "1024",
					}),
				),
				buildFrame(serverStreamID, frameData, 0, make([]byte, 512)),
				buildFrame(serverStreamID, frameData, flagEndStream, make([]byte, 512)),
			},
			response: &Response{
				Status: "206",
				Proto:  "HTTP/2",
				Size:   1109,
				Header: http.Header{
					"Content-Type":   []string{"video/mp4"},
					"Content-Length": []string{"1024"},
				},
			},
		},
		{
			name: "PaddedResponse",
			input: [][]byte{
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders|flagPadded,
					buildHeadersFramePayload(false, 16, map[string]string{
						":status": "201",
					}),
				),
				buildFrame(serverStreamID, frameData, flagEndStream|flagPadded,
					buildDataFramePayload(32, []byte("Created")),
				),
			},
			response: &Response{
				Status: "201",
				Proto:  "HTTP/2",
				Size:   88,
				Header: http.Header{},
			},
		},
		{
			name: "ServerPushResponse",
			input: [][]byte{
				buildFrame(serverStreamID, framePushPromise, flagEndHeaders,
					buildPushPromiseFramePayload(0, map[string]string{
						":method": "GET",
						":path":   "/style.css",
					}),
				),
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":status": "200",
					}),
				),
				buildFrame(serverStreamID, frameData, flagEndStream, []byte("OK")),
			},
			response: &Response{
				Status: "200",
				Proto:  "HTTP/2",
				Size:   77,
				Header: http.Header{},
			},
		},
		{
			name: "ErrorResponse",
			input: [][]byte{
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders|flagEndStream,
					buildHeadersFramePayload(false, 0, map[string]string{
						":status": "500",
						"x-error": "Internal Server Error",
					}),
				),
				buildFrame(serverStreamID, frameData, flagEndStream, []byte("Error")),
			},
			response: &Response{
				Status: "500",
				Proto:  "HTTP/2",
				Size:   67,
				Header: http.Header{
					"X-Error": []string{"Internal Server Error"},
				},
			},
		},
		{
			name: "CustomTrailers",
			input: [][]byte{
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":status":      "200",
						"content-type": "application/json",
					}),
				),
				buildFrame(serverStreamID, frameData, 0, []byte("OK")),
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders|flagEndStream,
					buildHeadersFramePayload(false, 0, map[string]string{
						"x-trailer-1": "value1",
						"x-trailer-2": "value2",
					}),
				),
			},
			trailerKeys: []string{"x-trailer-1", "x-trailer-2"},
			response: &Response{
				Status: "200",
				Proto:  "HTTP/2",
				Size:   113,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
			},
		},
		{
			name: "GRPCTrailers",
			input: [][]byte{
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						":status":      "200",
						"content-type": "application/grpc",
					}),
				),
				buildFrame(serverStreamID, frameData, 0, []byte("OK")),
				buildFrame(serverStreamID, frameHeaders, flagEndHeaders|flagEndStream,
					buildHeadersFramePayload(false, 0, map[string]string{
						"grpc-status":  "0",
						"grpc-message": "OK",
					}),
				),
			},
			trailerKeys: []string{"grpc-status", "grpc-message"},
			response: &Response{
				Status: "200",
				Proto:  "HTTP/2",
				Size:   105,
				Header: http.Header{
					"Content-Type": []string{"application/grpc"},
				},
			},
		},
	}

	var st socket.TupleRaw
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd := newStreamDecoder(2, st, 8080, NewHeaderFieldDecoder(tt.trailerKeys...))
			defer sd.Free()

			var got *role.Object
			var err error
			for _, chunk := range tt.input {
				got, err = sd.Decode(false, chunk, t0)
			}

			assert.NoError(t, err)
			obj := got.Obj.(*Response)

			assert.Equal(t, tt.response.Status, obj.Status)
			assert.Equal(t, tt.response.Size, obj.Size)
			assert.Equal(t, tt.response.Proto, obj.Proto)
			assert.Equal(t, tt.response.Header, obj.Header)
		})
	}
}

func TestStreamDecoderFailed(t *testing.T) {
	tests := []struct {
		name  string
		input [][]byte
	}{
		{
			name: "InvalidFrameType",
			input: [][]byte{
				buildFrame(clientStreamID, 0xFF, 0x00, []byte{}),
			},
		},
		{
			name: "MissingPseudoHeaders",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						"content-type": "text/plain",
					}),
				),
			},
		},
		{
			name: "InvalidPadding",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagPadded|flagEndHeaders,
					buildHeadersFramePayload(false, 300, map[string]string{
						":method": "GET",
						":path":   "/",
					}),
				),
			},
		},
		{
			name: "DataAfterReset",
			input: [][]byte{
				buildFrame(clientStreamID, frameRSTStream, 0, []byte{0x00, 0x00, 0x00, 0x01}), // PROTOCOL_ERROR
				buildFrame(clientStreamID, frameData, flagEndStream, []byte("data")),
			},
		},
		{
			name: "InvalidWindowUpdate",
			input: [][]byte{
				buildFrame(clientStreamID, frameWindowUpdate, 0, []byte{0x80, 0x00, 0x00, 0x00}), // 无效窗口增量
			},
		},
		{
			name: "HeaderOrderViolation",
			input: [][]byte{
				buildFrame(clientStreamID, frameHeaders, flagEndHeaders,
					buildHeadersFramePayload(false, 0, map[string]string{
						"content-type": "text/plain", // 常规头部在伪头部之前
						":method":      "GET",
						":path":        "/",
					}),
				),
			},
		},
	}

	var st socket.TupleRaw
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd := newStreamDecoder(1, st, 0, NewHeaderFieldDecoder())
			defer sd.Free()

			var objs *role.Object
			for _, chunk := range tt.input {
				objs, _ = sd.Decode(false, chunk, t0)
			}
			assert.Nil(t, objs)
		})
	}
}
