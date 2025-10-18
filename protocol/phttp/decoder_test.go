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

package phttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/splitio"
	"github.com/packetd/packetd/internal/zerocopy"
)

func normalizeProtocol(b []byte) []byte {
	b = bytes.TrimSpace(b)
	b = bytes.ReplaceAll(b, splitio.CharLF, splitio.CharCRLF)
	b = append(b, splitio.CharCRLF...)
	b = append(b, splitio.CharCRLF...)
	return b
}

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		request *Request
	}{
		{
			name: "GET request",
			input: normalizeProtocol([]byte(`
GET /index.html HTTP/1.1
Host: www.example.com
User-Agent: Gecko/20100101 Firefox/91.0
Accept: text/html
Accept-Encoding: gzip
Connection: keep-alive`)),
			request: &Request{
				Host:   "www.example.com",
				Method: http.MethodGet,
				URL:    "/index.html",
				Proto:  "HTTP/1.1",
				Header: http.Header{
					"User-Agent":      []string{"Gecko/20100101 Firefox/91.0"},
					"Accept":          []string{"text/html"},
					"Accept-Encoding": []string{"gzip"},
					"Connection":      []string{"keep-alive"},
				},
			},
		},
		{
			name: "GET with query params",
			input: normalizeProtocol([]byte(`
GET /search?q=golang&page=2 HTTP/1.1
Host: www.google.com
Accept-Language: en-US`)),
			request: &Request{
				Host:   "www.google.com",
				Method: http.MethodGet,
				URL:    "/search?q=golang&page=2",
				Proto:  "HTTP/1.1",
				Header: http.Header{
					"Accept-Language": []string{"en-US"},
				},
			},
		},
		{
			name: "GET with custom headers",
			input: normalizeProtocol([]byte(`
GET /private HTTP/1.1
Host: internal.example.com
X-Api-Key: 12345-67890
X-Custom-Header: value`)),
			request: &Request{
				Host:   "internal.example.com",
				Method: http.MethodGet,
				URL:    "/private",
				Proto:  "HTTP/1.1",
				Header: http.Header{
					"X-Api-Key":       []string{"12345-67890"},
					"X-Custom-Header": []string{"value"},
				},
			},
		},
		{
			name: "GET with absolute-uri",
			input: normalizeProtocol([]byte(`
GET http://absolute.example.com/path HTTP/1.1
Host: proxy.example.com`)),
			request: &Request{
				Host:   "absolute.example.com",
				Method: http.MethodGet,
				Proto:  "HTTP/1.1",
				URL:    "http://absolute.example.com/path",
				Size:   0,
				Header: http.Header{},
			},
		},
		{
			name: "GET with empty header value",
			input: normalizeProtocol([]byte(`
GET /empty-value HTTP/1.1
Host: empty.example.com
X-Empty:`)),
			request: &Request{
				Host:   "empty.example.com",
				Method: http.MethodGet,
				Proto:  "HTTP/1.1",
				URL:    "/empty-value",
				Size:   0,
				Header: http.Header{
					"X-Empty": []string{""},
				},
			},
		},
		{
			name: "GET unicode url",
			input: normalizeProtocol([]byte(`
GET /search?q=中文 HTTP/1.1
Host: unicode.example.com`)),
			request: &Request{
				Host:   "unicode.example.com",
				Method: http.MethodGet,
				URL:    "/search?q=中文",
				Proto:  "HTTP/1.1",
				Header: http.Header{},
			},
		},
		{
			name: "GET with duplicated headers",
			input: normalizeProtocol([]byte(`
GET /duplicate HTTP/1.1
Host: dup.example.com
X-Header: first
X-Header: second`)),
			request: &Request{
				Host:   "dup.example.com",
				Method: http.MethodGet,
				Proto:  "HTTP/1.1",
				URL:    "/duplicate",
				Size:   0,
				Header: http.Header{
					"X-Header": []string{"first", "second"},
				},
			},
		},
		{
			name: "GET with case-insensitive headers",
			input: normalizeProtocol([]byte(`
GET /case HTTP/1.1
HOST: case.example.com
User-AGENT: TestClient/1.0
aCcEpT: application/json`)),
			request: &Request{
				Host:   "case.example.com",
				Method: http.MethodGet,
				Proto:  "HTTP/1.1",
				URL:    "/case",
				Header: http.Header{
					"User-Agent": []string{"TestClient/1.0"},
					"Accept":     []string{"application/json"},
				},
			},
		},
		{
			name: "GET with multiple cookies",
			input: normalizeProtocol([]byte(`
GET /profile HTTP/1.1
Host: auth.example.com
Cookie: session=abc123; user=john
Cookie: preference=dark`)),
			request: &Request{
				Host:   "auth.example.com",
				Method: http.MethodGet,
				Proto:  "HTTP/1.1",
				URL:    "/profile",
				Header: http.Header{
					"Cookie": []string{"session=abc123; user=john", "preference=dark"},
				},
			},
		},
		{
			name: "POST with body",
			input: normalizeProtocol([]byte(`
POST /api/v1/users HTTP/1.1
Host: api.example.com
Content-Type: application/json
Content-Length: 28

{"name":"john","age":30}`)),
			request: &Request{
				Host:   "api.example.com",
				Method: http.MethodPost,
				URL:    "/api/v1/users",
				Proto:  "HTTP/1.1",
				Size:   28,
				Header: http.Header{
					"Content-Type":   []string{"application/json"},
					"Content-Length": []string{"28"},
				},
			},
		},
		{
			name: "POST with chunked body",
			input: normalizeProtocol([]byte(`
POST /chunked HTTP/1.1
Host: chunk.example.com
Transfer-Encoding: chunked

7
Packetd
9
Developer
7
Network
0`)),
			request: &Request{
				Host:    "chunk.example.com",
				Method:  http.MethodPost,
				Proto:   "HTTP/1.1",
				URL:     "/chunked",
				Size:    27,
				Chunked: true,
				Header:  http.Header{},
			},
		},
		{
			name: "POST multipart form data",
			input: normalizeProtocol([]byte(`
POST /upload HTTP/1.1
Host: upload.example.com
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Length: 300

------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="text"

text default
------WebKitFormBoundary7MA4YWxkTrZu0gW
Content-Disposition: form-data; name="file1"; filename="a.txt"
Content-Type: text/plain

Content of a.txt

------WebKitFormBoundary7MA4YWxkTrZu0gW--`)),
			request: &Request{
				Host:   "upload.example.com",
				Method: http.MethodPost,
				Proto:  "HTTP/1.1",
				URL:    "/upload",
				Size:   300,
				Header: http.Header{
					"Content-Type":   []string{"multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW"},
					"Content-Length": []string{"300"},
				},
			},
		},
		{
			name: "POST malformed query params",
			input: normalizeProtocol([]byte(`
GET /search?q=%%% HTTP/1.1
Host: malformed.example.com`)),
			request: &Request{
				Host:   "malformed.example.com",
				Method: http.MethodGet,
				URL:    "/search?q=%%%",
				Header: http.Header{},
			},
		},
		{
			name: "POST with gzip encoded body",
			input: normalizeProtocol([]byte(`
POST /compressed HTTP/1.1
Host: compress.example.com
Content-Encoding: gzip
Content-Length: 40

` + string([]byte{
				0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
				0xf2, 0x48, 0xcd, 0xc9, 0xc9, 0x57, 0x08, 0xcf, 0x2f, 0xca,
				0x49, 0x51, 0x04, 0x00, 0x00, 0x00, 0xff, 0xff, 0x03, 0x00,
				0x2b, 0x77, 0x13, 0xcb, 0x0f, 0x00, 0x00, 0x00,
			}))),
			request: &Request{
				Host:   "compress.example.com",
				Method: http.MethodPost,
				Proto:  "HTTP/1.1",
				URL:    ("/compressed"),
				Size:   40,
				Header: http.Header{
					"Content-Encoding": []string{"gzip"},
					"Content-Length":   []string{"40"},
				},
			},
		},
		{
			name: "PUT with binary body",
			input: normalizeProtocol([]byte(`
PUT /binary HTTP/1.1
Host: binary.example.com
Content-Type: application/octet-stream
Content-Length: 16

` + string([]byte{0x00, 0x01, 0x02, 0x03, 0x7f, 0x80, 0xfe, 0xff, 0x55, 0xaa, 0x00, 0x7e, 0x12, 0x34}))),
			request: &Request{
				Host:   "binary.example.com",
				Method: http.MethodPut,
				Proto:  "HTTP/1.1",
				URL:    ("/binary"),
				Size:   16,
				Header: http.Header{
					"Content-Type":   []string{"application/octet-stream"},
					"Content-Length": []string{"16"},
				},
			},
		},
		{
			name: "DElETE request",
			input: normalizeProtocol([]byte(`
DELETE /resource/123 HTTP/1.1
Host: api.example.com
Authorization: Bearer token123`)),
			request: &Request{
				Host:   "api.example.com",
				Method: http.MethodDelete,
				Proto:  "HTTP/1.1",
				URL:    ("/resource/123"),
				Size:   0,
				Header: http.Header{
					"Authorization": []string{"Bearer token123"},
				},
			},
		},
		{
			name: "HEAD request",
			input: normalizeProtocol([]byte(`
HEAD /status HTTP/1.1
Host: status.example.com`)),
			request: &Request{
				Host:   "status.example.com",
				Method: http.MethodHead,
				Proto:  "HTTP/1.1",
				URL:    ("/status"),
				Size:   0,
				Header: http.Header{},
			},
		},
		{
			name: "OPTIONS request",
			input: normalizeProtocol([]byte(`
OPTIONS * HTTP/1.1
Host: options.example.com`)),
			request: &Request{
				Host:   "options.example.com",
				Method: http.MethodOptions,
				Proto:  "HTTP/1.1",
				URL:    ("*"),
				Header: http.Header{},
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0, common.NewOptions())
			objs, err := d.Decode(zerocopy.NewBuffer(tt.input), t0)
			assert.NoError(t, err)

			req := objs[0].Obj.(*Request)
			assert.Equal(t, tt.request.Method, req.Method)
			assert.Equal(t, tt.request.URL, req.URL)
			assert.Equal(t, tt.request.Size, req.Size)
			assert.Equal(t, tt.request.Header, req.Header)
			assert.Equal(t, tt.request.Chunked, req.Chunked)
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		response *Response
	}{
		{
			name: "OK response",
			input: normalizeProtocol([]byte(`
HTTP/1.1 200 OK
Date: Wed, 18 Apr 2024 12:00:00 GMT
Server: Apache/2.4.1 (Unix)
Last-Modified: Wed, 18 Apr 2024 11:00:00 GMT
Content-Length: 0
Content-Type: text/html; charset=UTF-8`)),
			response: &Response{
				Proto:      "HTTP/1.1",
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type":   []string{"text/html; charset=UTF-8"},
					"Last-Modified":  []string{"Wed, 18 Apr 2024 11:00:00 GMT"},
					"Server":         []string{"Apache/2.4.1 (Unix)"},
					"Date":           []string{"Wed, 18 Apr 2024 12:00:00 GMT"},
					"Content-Length": []string{"0"},
				},
			},
		},
		{
			name: "OK with json body",
			input: normalizeProtocol([]byte(`
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 32

{"status":"success","data":{}}`)),
			response: &Response{
				Proto:      "HTTP/1.1",
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type":   []string{"application/json"},
					"Content-Length": []string{"32"},
				},
				Body: json.RawMessage(`{"status":"success","data":{}}`)},
		},

		{
			name: "Chunked transfer-encoding",
			input: normalizeProtocol([]byte(`
HTTP/1.1 200 OK
Transfer-Encoding: chunked
Content-Type: text/plain

7
packetd
9
Developer
7
Network
0
`)),
			response: &Response{
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Status:     "200 OK",
				Header: http.Header{
					"Content-Type": []string{"text/plain"},
				},
			},
		},
		{
			name: "Response compressed",
			input: normalizeProtocol([]byte(`
HTTP/1.1 200 OK
Content-Encoding: gzip
Content-Length: 40

` + string([]byte{
				0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
				0xf2, 0x48, 0xcd, 0xc9, 0xc9, 0x57, 0x08, 0xcf, 0x2f, 0xca,
				0x49, 0x51, 0x04, 0x00, 0x00, 0x00, 0xff, 0xff, 0x03, 0x00,
				0x2b, 0x77, 0x13, 0xcb, 0x0f, 0x00, 0x00, 0x00,
			}))),
			response: &Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header: http.Header{
					"Content-Encoding": []string{"gzip"},
					"Content-Length":   []string{"40"},
				},
			},
		},
		{
			name: "204 NoContent",
			input: normalizeProtocol([]byte(`
HTTP/1.1 204 No Content
Connection: close`)),
			response: &Response{
				StatusCode: http.StatusNoContent,
				Status:     "204 No Content",
				Close:      true,
				Header:     http.Header{},
			},
		},
		{
			name: "Custom status-code",
			input: normalizeProtocol([]byte(`
HTTP/1.1 299 Custom Warning
Content-Type: text/plain
Content-Length: 20

Custom status code`)),
			response: &Response{
				StatusCode: 299,
				Status:     "299 Custom Warning",
				Proto:      "HTTP/1.1",
				Header: http.Header{
					"Content-Type":   []string{"text/plain"},
					"Content-Length": []string{"20"},
				},
			},
		},
		{
			name: "Response with multiple-cookies",
			input: normalizeProtocol([]byte(`
HTTP/1.1 200 OK
Set-Cookie: session=abc123; Path=/
Set-Cookie: lang=en-US; Domain=example.com`)),
			response: &Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Close:      true,
				Header: http.Header{
					"Set-Cookie": []string{
						"session=abc123; Path=/",
						"lang=en-US; Domain=example.com",
					},
				},
			},
		},
		{
			name: "Redirect chain",
			input: normalizeProtocol([]byte(`
HTTP/1.1 302 Found
Location: /new-location
Content-Length: 0`)),
			response: &Response{
				StatusCode: http.StatusFound,
				Status:     "302 Found",
				Header: http.Header{
					"Location":       []string{"/new-location"},
					"Content-Length": []string{"0"},
				},
			},
		},
		{
			name: "OK with cache",
			input: normalizeProtocol([]byte(`
HTTP/1.1 200 OK
Cache-Control: max-age=3600, public
ETag: "737060cd8c284d8af7ad3082f209582d"`)),
			response: &Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Close:      true,
				Header: http.Header{
					"Cache-Control": []string{"max-age=3600, public"},
					"Etag":          []string{`"737060cd8c284d8af7ad3082f209582d"`},
				},
			},
		},
		{
			name: "Connection upgrade",
			input: normalizeProtocol([]byte(`
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade`)),
			response: &Response{
				StatusCode: http.StatusSwitchingProtocols,
				Status:     "101 Switching Protocols",
				Header: http.Header{
					"Upgrade":    []string{"websocket"},
					"Connection": []string{"Upgrade"},
				},
			},
		},
		{
			name: "MultiLine headers",
			input: normalizeProtocol([]byte(`
HTTP/1.1 200 OK
X-Multi-Line: this is a 
  multi-line 
  header value`)),
			response: &Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Close:      true,
				Header: http.Header{
					"X-Multi-Line": []string{"this is a multi-line header value"},
				},
			},
		},
		{
			name: "503 Service Unavailable",
			input: normalizeProtocol([]byte(`
HTTP/1.1 503 Service Unavailable
Retry-After: 3600`)),
			response: &Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "503 Service Unavailable",
				Close:      true,
				Header: http.Header{
					"Retry-After": []string{"3600"},
				},
			},
		},
		{
			name: "Duplicated headers",
			input: normalizeProtocol([]byte(`
HTTP/1.1 200 OK
X-Custom: first
X-Custom: second`)),
			response: &Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Close:      true,
				Header: http.Header{
					"X-Custom": []string{"first", "second"},
				},
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0, common.NewOptions())
			objs, err := d.Decode(zerocopy.NewBuffer(tt.input), t0)
			assert.NoError(t, err)

			req := objs[0].Obj.(*Response)
			assert.Equal(t, tt.response.StatusCode, req.StatusCode)
			assert.Equal(t, tt.response.Status, req.Status)
			assert.Equal(t, tt.response.Close, req.Close)
			assert.Equal(t, tt.response.Header, req.Header)
		})
	}
}

func TestDecodeFailed(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name: "Invalid Content-Length",
			input: normalizeProtocol([]byte(`
POST /path HTTP/1.1
Host: example.com
Content-Length: invalid`)),
		},
		{
			name: "Negative Content-Length",
			input: normalizeProtocol([]byte(`
POST /path HTTP/1.1
Host: example.com
Content-Length: -10`)),
		},
		{
			name: "Invalid transfer encoding",
			input: normalizeProtocol([]byte(`
POST /path HTTP/1.1
Host: example.com
Transfer-Encoding: invalid`)),
		},
		{
			name: "Incomplete status line",
			input: normalizeProtocol([]byte(`
HTTP/1.1
Content-Type: text/plain`)),
		},
		{
			name: "Non-numeric status code",
			input: normalizeProtocol([]byte(`
HTTP/1.1 ABC OK
Content-Type: text/plain`)),
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0, common.NewOptions())
			_, err := d.Decode(zerocopy.NewBuffer(tt.input), t0)
			assert.Error(t, err)
		})
	}
}
