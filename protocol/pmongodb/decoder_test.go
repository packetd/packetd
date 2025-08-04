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

package pmongodb

import (
	"encoding/binary"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/splitio"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol/role"
)

func TestDecodeSourceCommand(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  sourceCommand
	}{
		{
			name:  "Empty input",
			input: []byte{},
			want:  sourceCommand{},
		},
		{
			name: "Normal case with $db and find",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x02, 'f', 'i', 'n', 'd', 0x00,
				0x06, 0x00, 0x00, 0x00, 't', 'e', 's', 't', 0x00,
				0x02, '$', 'd', 'b', 0x00,
				0x06, 0x00, 0x00, 0x00, 'm', 'y', 'd', 'b', 0x00,
			},
			want: sourceCommand{
				source:     "mydb",
				collection: "test",
				cmdName:    "find",
				cmdValue:   "test",
			},
		},
		{
			name: "Multiple commands should pick first",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x02, 'i', 'n', 's', 'e', 'r', 't', 0x00,
				0x06, 0x00, 0x00, 0x00, 'c', 'o', 'l', 'l', 0x00,
				0x02, 'f', 'i', 'n', 'd', 0x00,
				0x06, 0x00, 0x00, 0x00, 'd', 'o', 'c', 0x00,
				0x02, 'n', 's', 0x00,
				0x0A, 0x00, 0x00, 0x00, 't', 'e', 's', 't', '.', 'c', 'o', 'l', 'l', 0x00,
			},
			want: sourceCommand{
				source:     "test.coll",
				collection: "coll",
				cmdName:    "insert",
				cmdValue:   "coll",
			},
		},
		{
			name: "No command found",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x02, 'n', 's', 0x00,
				0x0A, 0x00, 0x00, 0x00, 't', 'e', 's', 't', '.', 'c', 'o', 'l', 'l', 0x00,
			},
			want: sourceCommand{
				source: "test.coll",
			},
		},
		{
			name: "Mixed field order with nested documents",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x03, 'n', 'e', 's', 't', 'e', 'd', 0x00,
				0x20, 0x00, 0x00, 0x00,
				0x02, 'i', 'n', 'n', 'e', 'r', 'K', 0x00,
				0x08, 0x00, 0x00, 0x00, 'i', 'n', 'n', 'e', 'r', 'V', 0x00,
				0x00,
				0x02, 'u', 'p', 'd', 'a', 't', 'e', 0x00,
				0x0F, 0x00, 0x00, 0x00, 'c', 'o', 'l', 'l', 'e', 'c', 't', 'i', 'o', 'n', 0x00,
				0x02, '$', 'd', 'b', 0x00,
				0x10, 0x00, 0x00, 0x00, 'd', 'a', 't', 'a', 'b', 'a', 's', 'e', '_', 'n', 'a', 'm', 'e', 0x00,
			},
			want: sourceCommand{
				source:     "database_name",
				collection: "collection",
				cmdName:    "update",
				cmdValue:   "collection",
			},
		},
		{
			name: "Long string with special characters",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x02, 'd', 'e', 'l', 'e', 't', 'e', 0x00,
				0x80, 0x00, 0x00, 0x00,
				'v', 'a', 'l', 0x00, 0x01, 0x7F, 0xFF, 0x80,
				0x02, 'n', 's', 0x00,
				0x0F, 0x00, 0x00, 0x00, 't', 'e', 's', 't', '.', 's', 'p', 'e', 'c', 'i', 'a', 'l', 0x00,
			},
			want: sourceCommand{
				source:     "test.special",
				collection: "val",
				cmdName:    "delete",
				cmdValue:   "val",
			},
		},
		{
			name: "Incomplete BSON structure",
			input: []byte{
				0x2E, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00,
				0x2A, 0x00, 0x00, 0x00,
				0x02, 'f', 'i', 'n', 'd', 0x00,
				0x06, 0x00, 0x00, 0x00, 't',
			},
			want: sourceCommand{},
		},
		{
			name: "Both $db and ns present",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x02, '$', 'd', 'b', 0x00,
				0x06, 0x00, 0x00, 0x00, 'm', 'y', 'd', 'b', 0x00,
				0x02, 'n', 's', 0x00,
				0x0A, 0x00, 0x00, 0x00, 't', 'e', 's', 't', '.', 'c', 'o', 'l', 'l', 0x00,
				0x02, 'c', 'o', 'u', 'n', 't', 0x00,
				0x06, 0x00, 0x00, 0x00, 'd', 'o', 'c', 's', 0x00,
			},
			want: sourceCommand{
				source:     "mydb",
				collection: "docs",
				cmdName:    "count",
				cmdValue:   "docs",
			},
		},
		{
			name: "Multiple commands across BSON fields",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x02, 'f', 'i', 'n', 'd', 0x00,
				0x06, 0x00, 0x00, 0x00, 't', 'e', 's', 't', 0x00,
				0x02, 'u', 'p', 'd', 'a', 't', 'e', 0x00,
				0x08, 0x00, 0x00, 0x00, 'd', 'o', 'c', 's', 0x00,
				0x02, 'n', 's', 0x00,
				0x0A, 0x00, 0x00, 0x00, 't', 'e', 's', 't', '.', 'c', 'o', 'l', 'l', 0x00,
			},
			want: sourceCommand{
				source:     "test.coll",
				collection: "test",
				cmdName:    "find",
				cmdValue:   "test",
			},
		},
		{
			name: "Non-string type interference",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x01, 'i', 'n', 't', 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x02, 'c', 'r', 'e', 'a', 't', 'e', 0x00,
				0x06, 0x00, 0x00, 0x00, 'c', 'o', 'l', 'l', 0x00,
				0x10, '$', 'd', 'b', 0x00,
				0x01, 0x00, 0x00, 0x00,
			},
			want: sourceCommand{
				source:     "",
				collection: "coll",
				cmdName:    "create",
				cmdValue:   "coll",
			},
		},
		{
			name: "Multi-level nested documents",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x03, 'o', 'u', 't', 'e', 'r', 0x00,
				0x30, 0x00, 0x00, 0x00,
				0x03, 'i', 'n', 'n', 'e', 'r', 0x00,
				0x20, 0x00, 0x00, 0x00,
				0x02, 'c', 'o', 'm', 'm', 'a', 'n', 'd', 0x00,
				0x06, 0x00, 0x00, 0x00, 'v', 'a', 'l', 0x00,
				0x00,
				0x02, 'm', 'a', 'p', 'R', 'e', 'd', 'u', 'c', 'e', 0x00,
				0x06, 0x00, 0x00, 0x00, 'j', 'o', 'b', 0x00,
				0x02, '$', 'd', 'b', 0x00,
				0x06, 0x00, 0x00, 0x00, 'a', 'n', 'a', 'l', 'y', 't', 'i', 'c', 's', 0x00,
			},
			want: sourceCommand{
				source:     "analytics",
				collection: "job",
				cmdName:    "mapReduce",
				cmdValue:   "job",
			},
		},
		{
			name: "Only source",
			input: []byte{
				0xFF, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00,
				0x26, 0x00, 0x00, 0x00,
				0x02, 'f', 'i', 'n', 'd', 0x00,
				0xFF, 0xFF, 0xFF, 0xFF,
				't', 'e', 's', 't',
				0x02, 'n', 's', 0x00,
				0x0A, 0x00, 0x00, 0x00, 't', 'e', 's', 't', '.', 'c', 'o', 'l', 'l', 0x00,
			},
			want: sourceCommand{
				source:  "test.coll",
				cmdName: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeSourceCommand(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeOkCode(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  okCode
	}{
		{
			name: "Basic ok response",
			input: bsonDocBytes(bson.D{
				{Key: "ok", Value: 1.0},
			}),
			want: okCode{ok: 1.0},
		},
		{
			name: "Error response with code",
			input: bsonDocBytes(bson.D{
				{Key: "ok", Value: 0.0},
				{Key: "code", Value: 11000},
			}),
			want: okCode{ok: 0.0, code: 11000},
		},
		{
			name: "Only code field",
			input: bsonDocBytes(bson.D{
				{Key: "code", Value: 13},
				{Key: "errmsg", Value: "Unauthorized"},
			}),
			want: okCode{code: 13},
		},
		{
			name: "Large ok value",
			input: bsonDocBytes(bson.D{
				{Key: "ok", Value: 123456.789},
			}),
			want: okCode{ok: 123456.789},
		},
		{
			name: "Missing ok field",
			input: bsonDocBytes(bson.D{
				{Key: "n", Value: 1},
				{Key: "nModified", Value: 1},
			}),
			want: okCode{},
		},
		{
			name: "Invalid ok type",
			input: bsonDocBytes(bson.D{
				{Key: "ok", Value: "not a number"},
				{Key: "code", Value: 123},
			}),
			want: okCode{code: 123},
		},
		{
			name: "Invalid code type",
			input: bsonDocBytes(bson.D{
				{Key: "ok", Value: 1.0},
				{Key: "code", Value: "not a number"},
			}),
			want: okCode{ok: 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeOkCode(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func bsonDocBytes(doc bson.D) []byte {
	b, _ := bson.Marshal(doc)
	return b
}

func buildMongoDBMessage(doc bson.D, size int, rspTo uint32) [][]byte {
	payload := bsonDocBytes(doc)

	// OP_MSG 消息结构:
	// 标准消息头(16字节) + 标志位(4字节) + 0x00(1字节) + 主文档 + 可选文档序列
	// 这里我们只包含主文档部分

	// 计算消息总长度
	totalLength := 21 + len(payload)
	msg := make([]byte, totalLength)
	pos := 0

	// 设置标准消息头
	// messageLength
	binary.LittleEndian.PutUint32(msg[pos:pos+4], uint32(totalLength))
	pos += 4

	// requestId
	binary.LittleEndian.PutUint32(msg[pos:pos+4], 0x01)
	pos += 4

	// responseTo
	binary.LittleEndian.PutUint32(msg[pos:pos+4], rspTo)
	pos += 4

	// opcode
	binary.LittleEndian.PutUint32(msg[pos:pos+4], uint32(opcodeMsg))
	pos += 4

	// 设置标志位 (0 表示无特殊标志)
	binary.LittleEndian.PutUint32(msg[pos:pos+4], 0)
	pos += 4

	// 设置文档类型 (0x00 表示 BodySection)
	msg[pos] = 0x00
	pos += 1

	copy(msg[pos:], payload)
	return splitio.SplitChunk(msg, size)
}

func buildNDocs(n int) bson.D {
	var doc bson.D
	for i := 0; i < n; i++ {
		doc = append(doc, bson.E{Key: "key" + strconv.Itoa(i), Value: "value" + strconv.Itoa(i)})
	}
	return doc
}

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   [][]byte
		request *Request
	}{
		{
			name: "Aggregate command with pipeline",
			input: buildMongoDBMessage(bson.D{
				{Key: "aggregate", Value: "orders"},
				{Key: "pipeline", Value: bson.A{
					bson.D{
						{Key: "$match", Value: bson.D{{Key: "status", Value: "completed"}}},
					},
					bson.D{
						{Key: "$group", Value: bson.D{
							{Key: "_id", Value: "$customer"},
							{Key: "total", Value: bson.D{{Key: "$sum", Value: "$amount"}}},
						}},
					},
				}},
				{Key: "$db", Value: "sales"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "sales",
				CmdName:  "aggregate",
				CmdValue: "orders",
				Size:     191,
			},
		},
		{
			name: "Create index command",
			input: buildMongoDBMessage(bson.D{
				{Key: "createIndexes", Value: "products"},
				{Key: "indexes", Value: bson.A{bson.D{
					{Key: "key", Value: bson.D{{Key: "name", Value: 1}}},
					{Key: "name", Value: "name_index"},
				}}},
				{Key: "$db", Value: "inventory"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "inventory",
				CmdName:  "createIndexes",
				CmdValue: "products",
				Size:     136,
			},
		},
		{
			name: "Bulk write operation",
			input: buildMongoDBMessage(bson.D{
				{Key: "insert", Value: "users"},
				{Key: "documents", Value: bson.A{
					bson.D{{Key: "name", Value: "Bob"}, {Key: "age", Value: 25}},
					bson.D{{Key: "name", Value: "Charlie"}, {Key: "age", Value: 35}},
				}},
				{Key: "ordered", Value: false},
				{Key: "$db", Value: "accounts"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "accounts",
				CmdName:  "insert",
				CmdValue: "users",
				Size:     154,
			},
		},
		{
			name: "Find with projection",
			input: buildMongoDBMessage(bson.D{
				{Key: "find", Value: "employees"},
				{Key: "filter", Value: bson.D{{Key: "department", Value: "IT"}}},
				{Key: "projection", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}}},
				{Key: "$db", Value: "hr"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "hr",
				CmdName:  "find",
				CmdValue: "employees",
				Size:     126,
			},
		},
		{
			name: "Distinct values",
			input: buildMongoDBMessage(bson.D{
				{Key: "distinct", Value: "products"},
				{Key: "key", Value: "category"},
				{Key: "query", Value: bson.D{{Key: "price", Value: bson.D{{Key: "$gt", Value: 100}}}}},
				{Key: "$db", Value: "ecommerce"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "ecommerce",
				CmdName:  "distinct",
				CmdValue: "products",
				Size:     119,
			},
		},
		{
			name: "Create collection",
			input: buildMongoDBMessage(bson.D{
				{Key: "create", Value: "auditLog"},
				{Key: "capped", Value: true},
				{Key: "size", Value: 1000000},
				{Key: "$db", Value: "admin"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "admin",
				CmdName:  "create",
				CmdValue: "auditLog",
				Size:     81,
			},
		},
		{
			name: "List database doubleType",
			input: buildMongoDBMessage(bson.D{
				{Key: "listDatabases", Value: float64(1)},
				{Key: "$db", Value: "admin"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "admin",
				CmdName:  "listDatabases",
				CmdValue: "1",
				Size:     64,
			},
		},
		{
			name: "List database int32Type",
			input: buildMongoDBMessage(bson.D{
				{Key: "listDatabases", Value: int32(1)},
				{Key: "$db", Value: "admin"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "admin",
				CmdName:  "listDatabases",
				CmdValue: "1",
				Size:     60,
			},
		},
		{
			name: "Insert command with document",
			input: buildMongoDBMessage(bson.D{
				{Key: "insert", Value: "users"},
				{Key: "documents", Value: bson.A{bson.D{{Key: "name", Value: "Alice"}, {Key: "age", Value: 30}}}},
				{Key: "$db", Value: "mydb"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "mydb",
				CmdName:  "insert",
				CmdValue: "users",
				Size:     107,
			},
		},
		{
			name: "Update command with complex query",
			input: buildMongoDBMessage(bson.D{
				{Key: "update", Value: "products"},
				{Key: "updates", Value: bson.A{bson.D{
					{Key: "q", Value: bson.D{{Key: "category", Value: "electronics"}}},
					{Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "price", Value: 999.99}}}}},
				}}},
				{Key: "$db", Value: "shop"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "shop",
				CmdName:  "update",
				CmdValue: "products",
				Size:     151,
			},
		},
		{
			name: "Delete command with multiple conditions",
			input: buildMongoDBMessage(bson.D{
				{Key: "delete", Value: "logs"},
				{Key: "deletes", Value: bson.A{bson.D{
					{Key: "q", Value: bson.D{
						{Key: "level", Value: "debug"},
						{Key: "timestamp", Value: bson.D{{Key: "$lt", Value: time.Now().Add(-24 * time.Hour)}}},
					}},
					{Key: "limit", Value: 0},
				}}},
				{Key: "$db", Value: "monitoring"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "monitoring",
				CmdName:  "delete",
				CmdValue: "logs",
				Size:     150,
			},
		},
		{
			name: "Insert large document",
			input: buildMongoDBMessage(bson.D{
				{Key: "insert", Value: "users"},
				{Key: "documents", Value: bson.A{buildNDocs(10000)}},
				{Key: "$db", Value: "mydb"},
			}, common.ReadWriteBlockSize, 0),
			request: &Request{
				Source:   "mydb",
				CmdName:  "insert",
				CmdValue: "users",
				Size:     227862,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0, common.NewOptions())
			var err error
			var objs []*role.Object
			for _, input := range tt.input {
				objs, err = d.Decode(zerocopy.NewBuffer(input), t0)
			}
			assert.NoError(t, err)

			obj, ok := objs[0].Obj.(*Request)
			assert.True(t, ok)
			assert.Equal(t, tt.request.Source, obj.Source)
			assert.Equal(t, tt.request.CmdName, obj.CmdName)
			assert.Equal(t, tt.request.CmdValue, obj.CmdValue)
			assert.Equal(t, tt.request.Size, obj.Size)
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    [][]byte
		response *Response
	}{
		{
			name:  "Empty response",
			input: buildMongoDBMessage(bson.D{}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 26,
			},
		},
		{
			name: "Find command response with single document",
			input: buildMongoDBMessage(bson.D{
				{Key: "cursor", Value: bson.D{
					{Key: "id", Value: int64(123456789)},
					{Key: "ns", Value: "testdb.collection"},
					{Key: "firstBatch", Value: bson.A{bson.D{{Key: "_id", Value: "abc123"}, {Key: "name", Value: "test"}}}},
				}},
				{Key: "ok", Value: 1.0},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 145,
				Ok:   1,
			},
		},
		{
			name: "Insert command response",
			input: buildMongoDBMessage(bson.D{
				{Key: "n", Value: 1},
				{Key: "ok", Value: 1.0},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 45,
				Ok:   1,
			},
		},
		{
			name: "Error response",
			input: buildMongoDBMessage(bson.D{
				{Key: "ok", Value: 0.0},
				{Key: "errmsg", Value: "Duplicate key error"},
				{Key: "code", Value: 11000},
				{Key: "codeName", Value: "DuplicateKey"},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 107,
				Ok:   0,
				Code: 11000,
			},
		},
		{
			name: "Aggregate command response with multiple batches",
			input: buildMongoDBMessage(bson.D{
				{Key: "cursor", Value: bson.D{
					{Key: "id", Value: int64(987654321)},
					{Key: "ns", Value: "analytics.events"},
					{Key: "firstBatch", Value: bson.A{
						bson.D{{Key: "event", Value: "click"}, {Key: "count", Value: 42}},
						bson.D{{Key: "event", Value: "view"}, {Key: "count", Value: 100}},
					}},
				}},
				{Key: "ok", Value: 1.0},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 176,
				Ok:   1.0,
			},
		},
		{
			name: "Count command response",
			input: buildMongoDBMessage(bson.D{
				{Key: "n", Value: 42},
				{Key: "ok", Value: 1.0},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 45,
				Ok:   1.0,
			},
		},
		{
			name: "Large response document",
			input: buildMongoDBMessage(bson.D{
				{Key: "cursor", Value: bson.D{
					{Key: "id", Value: int64(0)},
					{Key: "ns", Value: "bigdata.collection"},
					{Key: "firstBatch", Value: bson.A{buildNDocs(10000)}},
				}},
				{Key: "ok", Value: 1.0},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 227895,
				Ok:   1.0,
			},
		},
		{
			name: "GetMore command response",
			input: buildMongoDBMessage(bson.D{
				{Key: "cursor", Value: bson.D{
					{Key: "id", Value: int64(123456789)},
					{Key: "ns", Value: "testdb.collection"},
					{Key: "nextBatch", Value: bson.A{
						bson.D{{Key: "_id", Value: "def456"}, {Key: "name", Value: "test2"}},
						bson.D{{Key: "_id", Value: "ghi789"}, {Key: "name", Value: "test3"}},
					}},
				}},
				{Key: "ok", Value: 1.0},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 185,
				Ok:   1.0,
			},
		},
		{
			name: "Update command response with write concern",
			input: buildMongoDBMessage(bson.D{
				{Key: "n", Value: 1},
				{Key: "nModified", Value: 1},
				{Key: "ok", Value: 1.0},
				{Key: "writeConcern", Value: bson.D{
					{Key: "w", Value: "majority"},
					{Key: "wtimeout", Value: 5000},
				}},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 109,
				Ok:   1.0,
			},
		},
		{
			name: "Bulk write response",
			input: buildMongoDBMessage(bson.D{
				{Key: "nInserted", Value: 2},
				{Key: "nUpserted", Value: 0},
				{Key: "nMatched", Value: 0},
				{Key: "nModified", Value: 0},
				{Key: "nRemoved", Value: 0},
				{Key: "upserted", Value: bson.A{}},
				{Key: "writeErrors", Value: bson.A{}},
				{Key: "writeConcernError", Value: nil},
				{Key: "ok", Value: 1.0},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 163,
				Ok:   1.0,
			},
		},
		{
			name: "Command response with nested documents",
			input: buildMongoDBMessage(bson.D{
				{Key: "stats", Value: bson.D{
					{Key: "size", Value: 1024},
					{Key: "count", Value: 42},
					{Key: "avgObjSize", Value: 24},
					{Key: "storageSize", Value: 2048},
				}},
				{Key: "ok", Value: 1.0},
			}, common.ReadWriteBlockSize, 1),
			response: &Response{
				Size: 104,
				Ok:   1.0,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := common.NewOptions()
			opts.Merge(OptEnableResponseCode, true)
			d := NewDecoder(st, 0, opts)
			var err error
			var objs []*role.Object
			for _, input := range tt.input {
				objs, err = d.Decode(zerocopy.NewBuffer(input), t0)
			}
			assert.NoError(t, err)

			obj, ok := objs[0].Obj.(*Response)
			assert.True(t, ok)
			assert.Equal(t, tt.response.Size, obj.Size)
			assert.Equal(t, tt.response.Ok, obj.Ok)
			assert.Equal(t, tt.response.Code, obj.Code)
		})
	}
}

func buildWithHeader(payload []byte) []byte {
	header := make([]byte, 16)
	binary.LittleEndian.PutUint32(header, uint32(16+len(payload)))
	binary.LittleEndian.PutUint32(header[4:], 1)
	binary.LittleEndian.PutUint32(header[8:], 0)
	binary.LittleEndian.PutUint32(header[12:], uint32(opcodeMsg))
	return append(header, payload...)
}

func TestDecodeFailed(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		isErr bool
	}{
		{
			name:  "Empty input",
			input: []byte{},
			isErr: true,
		},
		{
			name: "Short message length",
			input: buildWithHeader([]byte{
				0x01, 0x00, 0x00, 0x00,
				0x00,
				0x00,
			}),
		},
		{
			name: "Invalid BSON document",
			input: buildWithHeader([]byte{
				0x00, 0x00, 0x00, 0x00,
				0x00,
				0x05, 0x00, 0x00, 0x00,
				0x00,
			}),
		},
		{
			name: "Invalid BSON type in document",
			input: buildWithHeader([]byte{
				0x00, 0x00, 0x00, 0x00,
				0x00,
				0x0C, 0x00, 0x00, 0x00,
				0xFF,
				'k', 'e', 'y', 0x00,
			}),
		},
		{
			name: "Truncated BSON document",
			input: buildWithHeader([]byte{
				0x00, 0x00, 0x00, 0x00,
				0x00,
				0x10, 0x00, 0x00, 0x00,
				0x02, 'k', 'e', 'y', 0x00,
				0x04, 0x00, 0x00, 0x00,
				'v', 'a', 'l',
			}),
		},
		{
			name: "Invalid section type",
			input: buildWithHeader([]byte{
				0x00, 0x00, 0x00, 0x00,
				0xFF,
			}),
		},
		{
			name: "Incomplete message header",
			input: []byte{
				0x10, 0x00, 0x00, 0x00,
				0x01, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00,
			},
			isErr: true,
		},
		{
			name: "Random garbage data with header",
			input: func() []byte {
				header := make([]byte, 16)
				binary.LittleEndian.PutUint32(header, 1024)
				binary.LittleEndian.PutUint32(header[12:], uint32(opcodeMsg))
				body := []byte{
					0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0,
					0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
				}
				return append(header, body...)
			}(),
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0, common.NewOptions())
			objs, err := d.Decode(zerocopy.NewBuffer(tt.input), t0)
			if tt.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Nil(t, objs)
		})
	}
}
