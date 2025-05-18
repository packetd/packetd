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

package pmysql

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol/role"
)

func buildQueryPacket(query string) [][]byte {
	var buf bytes.Buffer
	payload := make([]byte, 0, len(query)+1)

	payload = append(payload, cmdQuery)
	payload = append(payload, query...)
	chunkSize := len(payload)

	// Header
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, uint32(chunkSize))
	header[3] = 0x00
	buf.Write(header[:4])

	// Payload
	buf.Write(payload[:chunkSize])

	const maxBytes = 4096
	var lst [][]byte
	for {
		b := make([]byte, maxBytes)
		n, err := buf.Read(b)
		if err == io.EOF {
			break
		}
		lst = append(lst, b[:n])
	}
	return lst
}

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   [][]byte
		request *Request
	}{
		{
			name:  "SELECT",
			input: buildQueryPacket("SELECT * FROM users WHERE id = 1;"),
			request: &Request{
				Command:   "QUERY",
				Statement: "SELECT * FROM users WHERE id = 1;",
				Size:      38,
			},
		},
		{
			name:  "SELECT with comment",
			input: buildQueryPacket("SELECT * FROM users WHERE id = 1; -- This is a comment"),
			request: &Request{
				Command:   "QUERY",
				Statement: "SELECT * FROM users WHERE id = 1; -- This is a comment",
				Size:      59,
			},
		},
		{
			name:  "INSERT",
			input: buildQueryPacket("INSERT INTO users (name, age) VALUES ('John', 25);"),
			request: &Request{
				Command:   "QUERY",
				Statement: "INSERT INTO users (name, age) VALUES ('John', 25);",
				Size:      55,
			},
		},
		{
			name:  "UPDATE",
			input: buildQueryPacket("UPDATE users SET name = 'Jane' WHERE id = 2;"),
			request: &Request{
				Command:   "QUERY",
				Statement: "UPDATE users SET name = 'Jane' WHERE id = 2;",
				Size:      49,
			},
		},
		{
			name:  "DELETE",
			input: buildQueryPacket("DELETE FROM users WHERE id = 3;"),
			request: &Request{
				Command:   "QUERY",
				Statement: "DELETE FROM users WHERE id = 3;",
				Size:      36,
			},
		},
		{
			name:  "CREATE TABLE",
			input: buildQueryPacket("CREATE TABLE products (id INT, name VARCHAR(255));"),
			request: &Request{
				Command:   "QUERY",
				Statement: "CREATE TABLE products (id INT, name VARCHAR(255));",
				Size:      55,
			},
		},
		{
			name:  "ALTER TABLE",
			input: buildQueryPacket("ALTER TABLE users ADD COLUMN email VARCHAR(100);"),
			request: &Request{
				Command:   "QUERY",
				Statement: "ALTER TABLE users ADD COLUMN email VARCHAR(100);",
				Size:      53,
			},
		},
		{
			name:  "PREPARE STATEMENT",
			input: buildQueryPacket("PREPARE stmt1 FROM 'SELECT * FROM users WHERE id = ?';"),
			request: &Request{
				Command:   "QUERY",
				Statement: "PREPARE stmt1 FROM 'SELECT * FROM users WHERE id = ?';",
				Size:      59,
			},
		},
		{
			name:  "EXECUTE STATEMENT",
			input: buildQueryPacket("EXECUTE stmt1 USING @id;"),
			request: &Request{
				Command:   "QUERY",
				Statement: "EXECUTE stmt1 USING @id;",
				Size:      29,
			},
		},
		{
			name:  "SET VARIABLE",
			input: buildQueryPacket("SET @name = 'test_user';"),
			request: &Request{
				Command:   "QUERY",
				Statement: "SET @name = 'test_user';",
				Size:      29,
			},
		},
		{
			name:  "CALL PROCEDURE",
			input: buildQueryPacket("CALL GetUserDetails(123);"),
			request: &Request{
				Command:   "QUERY",
				Statement: "CALL GetUserDetails(123);",
				Size:      30,
			},
		},
		{
			name:  "MULTIBYTE CHARACTERS",
			input: buildQueryPacket("SELECT * FROM 用户表 WHERE 名称 = '测试';"),
			request: &Request{
				Command:   "QUERY",
				Statement: "SELECT * FROM 用户表 WHERE 名称 = '测试';",
				Size:      53,
			},
		},
		{
			name:  "LONG QUERY",
			input: buildQueryPacket("SELECT " + string(make([]byte, 5000)) + " FROM huge_table;"),
			request: &Request{
				Command:   "QUERY",
				Statement: "SELECT " + string(make([]byte, 1017)),
				Size:      5029,
			},
		},
		{
			name:  "BINARY DATA",
			input: buildQueryPacket("INSERT INTO files (data) VALUES (x'89504E470D0A1A0A');"),
			request: &Request{
				Command:   "QUERY",
				Statement: "INSERT INTO files (data) VALUES (x'89504E470D0A1A0A');",
				Size:      59,
			},
		},
		{
			name:  "LOCK TABLES",
			input: buildQueryPacket("LOCK TABLES users WRITE;"),
			request: &Request{
				Command:   "QUERY",
				Statement: "LOCK TABLES users WRITE;",
				Size:      29,
			},
		},
		{
			name:  "ANALYZE TABLE",
			input: buildQueryPacket("ANALYZE TABLE users;"),
			request: &Request{
				Command:   "QUERY",
				Statement: "ANALYZE TABLE users;",
				Size:      25,
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
			for _, input := range tt.input {
				objs, err = d.Decode(zerocopy.NewBuffer(input), t0)
			}
			assert.NoError(t, err)

			obj, ok := objs[0].Obj.(*Request)
			assert.True(t, ok)
			assert.Equal(t, tt.request.Command, obj.Command)
			assert.Equal(t, tt.request.Statement, obj.Statement)
			assert.Equal(t, tt.request.Size, obj.Size)
		})
	}
}

var eofPacket = []byte{0xfe, 0x00, 0x00, 0x02, 0x00}

func writePacket(buf *bytes.Buffer, payload []byte) {
	length := uint32(len(payload))
	header := make([]byte, 4)
	header[0] = byte(length)
	header[1] = byte(length >> 8)
	header[2] = byte(length >> 16)
	header[3] = 0
	buf.Write(header)
	buf.Write(payload)
}

func buildResultSetPacket(n int) [][]byte {
	var buf bytes.Buffer

	// 1) 列数量 (2 列)
	writePacket(&buf, []byte{0x02})

	// 2) 列定义 id (INT)
	writePacket(&buf, []byte{
		0x03, 0x64, 0x65, 0x66, // def
		0x00, 0x00, 0x00, 0x01, 0x69, 0x64, // Table/id
		0x00,             // 结束标识
		0x0c, 0x3f, 0x00, // 列属性
		0x04, 0x00, 0x00, 0x00, 0x03, // INT 类型
	})

	// 3) 列定义 name (VARCHAR)
	writePacket(&buf, []byte{
		0x03, 0x64, 0x65, 0x66, // def
		0x00, 0x00, 0x00, 0x01, 0x6e, 0x61, 0x6d, 0x65, // Table/name
		0x00,             // 结束标识
		0x0c, 0x21, 0x00, // 列属性
		0xfd, 0x01, 0x00, 0x1f, 0x00, 0x00, // VARCHAR 类型
	})

	// 4) EOF 列定义结束
	writePacket(&buf, eofPacket)

	// 5) 生成 n 行数据
	for i := 1; i <= n; i++ {
		var rowData bytes.Buffer
		rowData.Write([]byte{0x04})
		binary.Write(&rowData, binary.LittleEndian, int32(i))
		name := fmt.Sprintf("Row %d", i)
		rowData.Write([]byte{byte(len(name))})
		rowData.WriteString(name)
		writePacket(&buf, rowData.Bytes()) // 序列号递增
	}

	// 6) 最终 EOF 包
	writePacket(&buf, eofPacket)

	const maxBytes = 4096
	var lst [][]byte
	for {
		b := make([]byte, maxBytes)
		n, err := buf.Read(b)
		if err == io.EOF {
			break
		}
		lst = append(lst, b[:n])
	}
	return lst
}

func buildErrorPacket(code uint16, sqlState string, message string) []byte {
	var errPacket bytes.Buffer
	errPacket.WriteByte(0xFF)

	binary.Write(&errPacket, binary.LittleEndian, code)

	errPacket.WriteString(sqlState) // #HY000
	errPacket.WriteString(message)

	var buf bytes.Buffer
	writePacket(&buf, errPacket.Bytes())
	return buf.Bytes()
}

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name     string
		inputs   [][]byte
		response *Response
	}{
		{
			name:   "OKPacket",
			inputs: [][]byte{{0x07, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x02, 0x00, 0x00, 0x00}},
			response: &Response{
				Size: 11,
				Packet: &OKPacket{
					AffectedRows: 1,
					LastInsertID: 0,
					Status:       2,
					Warnings:     0,
				},
			},
		},
		{
			name:   "OKPacketWithLastInsertID",
			inputs: [][]byte{{0x07, 0x00, 0x00, 0x01, 0x00, 0x05, 0x00, 0x02, 0x05, 0x00, 0x00}},
			response: &Response{
				Size: 11,
				Packet: &OKPacket{
					AffectedRows: 5,
					LastInsertID: 0,
					Status:       2,
					Warnings:     5,
				},
			},
		},
		{
			name:   "ErrorPacket",
			inputs: [][]byte{buildErrorPacket(1064, "#HY000", "Table not found")},
			response: &Response{
				Size: 28,
				Packet: &ErrorPacket{
					ErrCode:  1064,
					ErrMsg:   "Table not found",
					SQLState: "HY000",
				},
			},
		},
		{
			name:   "ResultSetPacket",
			inputs: buildResultSetPacket(100),
			response: &Response{
				Size: 1664,
				Packet: &ResultSetPacket{
					Rows: 100,
				},
			},
		},
		{
			name:   "ResultSetPacket",
			inputs: buildResultSetPacket(10000),
			response: &Response{
				Size: 178966,
				Packet: &ResultSetPacket{
					Rows: 10000,
				},
			},
		},
		{
			name:   "EmptyResultSet",
			inputs: buildResultSetPacket(0),
			response: &Response{
				Size: 72,
				Packet: &ResultSetPacket{
					Rows: 0,
				},
			},
		},
		{
			name:   "ErrorPacketWithLongMessage",
			inputs: [][]byte{buildErrorPacket(1105, "#HY000", strings.Repeat("Error message ", 50))},
			response: &Response{
				Size: 713,
				Packet: &ErrorPacket{
					ErrCode:  1105,
					ErrMsg:   strings.Repeat("Error message ", 50)[:maxErrMsgSize],
					SQLState: "HY000",
				},
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
			name:   "MalformedOKPacket",
			inputs: [][]byte{{0x03, 0x00, 0x00}}, // Incomplete packet
		},
		{
			name:   "AuthSwitchRequest",
			inputs: [][]byte{{0x01, 0x00, 0x00, 0x00, 0xfe}},
		},
		{
			name:   "LocalInfileRequest",
			inputs: [][]byte{{0x01, 0x00, 0x00, 0x00, 0xff}},
		},
		{
			name:   "TruncatedErrorPacket",
			inputs: [][]byte{{0xff, 0x15, 0x04}},
		},
		{
			name:   "InvalidProtocolVersion",
			inputs: [][]byte{{0x0a, 0x35, 0x2e, 0x31, 0x2e, 0x37, 0x32, 0x00}},
		},
		{
			name:   "InvalidSSLRequest",
			inputs: [][]byte{{0x20, 0x00, 0x00, 0x00, 0x01}},
		},
		{
			name:   "MalformedSTMTExecute",
			inputs: [][]byte{{0x17, 0x00, 0x00, 0x00}},
		},
		{
			name:   "InvalidCompressedPacket",
			inputs: [][]byte{{0x00, 0x00, 0x00, 0x00, 0x01}},
		},
		{
			name:   "UnexpectedOKPacket",
			inputs: [][]byte{{0x00, 0x00, 0x00, 0x02, 0x00}},
		},
		{
			name:   "MixedEncodingPacket",
			inputs: [][]byte{{0x03, 0x00, 0x00, 0x00, 0xfe, 0x00, 0xff, 0xfd}},
		},
		{
			name:   "InvalidCharacterSet",
			inputs: [][]byte{{0x2c, 0x00, 0x00, 0x00, 0x21, 0xff, 0xff}},
		},
		{
			name:   "ProtocolMismatch",
			inputs: [][]byte{{0x03, 0x00, 0x00, 0x00, 0x02}},
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

func TestDecodeLenEncodedInteger(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		val   int
		tail  []byte
		ok    bool
	}{
		{
			name:  "Empty input",
			input: nil,
			val:   0,
			tail:  nil,
			ok:    false,
		},
		{
			name:  "1Bytes",
			input: []byte{0x7F, 0x01, 0x02},
			val:   127,
			tail:  []byte{0x01, 0x02},
			ok:    true,
		},
		{
			name:  "NULL (0xfb)",
			input: []byte{0xfb, 0x01, 0x02},
			val:   0,
			tail:  []byte{0x01, 0x02},
			ok:    true,
		},
		{
			name:  "2Bytes (0xfc)",
			input: []byte{0xfc, 0xFF, 0xFF, 0x01},
			val:   65535,
			tail:  []byte{0x01},
			ok:    true,
		},
		{
			name:  "2Bytes incomplete",
			input: []byte{0xfc, 0xFF},
			val:   0,
			tail:  []byte{0xfc, 0xFF},
			ok:    false,
		},
		{
			name:  "3Bytes (0xfd)",
			input: []byte{0xfd, 0xFF, 0xFF, 0xFF, 0x01},
			val:   16777215,
			tail:  []byte{0x01},
			ok:    true,
		},
		{
			name:  "3Bytes incomplete",
			input: []byte{0xfd, 0xFF, 0xFF},
			val:   0,
			tail:  []byte{0xfd, 0xFF, 0xFF},
			ok:    false,
		},
		{
			name:  "8Bytes (0xfe)",
			input: []byte{0xfe, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF},
			val:   1,
			tail:  []byte{0xFF},
			ok:    true,
		},
		{
			name:  "8Bytes incomplete",
			input: []byte{0xfe, 0xFF, 0xFF, 0xFF, 0xFF},
			val:   0,
			tail:  []byte{0xfe, 0xFF, 0xFF, 0xFF, 0xFF},
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, tail, ok := decodeLenEncodedInteger(tt.input)
			assert.Equal(t, tt.val, val)
			assert.Equal(t, tt.tail, tail)
			assert.Equal(t, tt.ok, ok)
		})
	}
}
