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

package pdns

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/zerocopy"
)

func buildQuestionMessage(q dnsmessage.Question) []byte {
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID:    0x01,
		RCode: dnsmessage.RCodeSuccess,
	})
	_ = builder.StartQuestions()
	_ = builder.Question(q)
	msg, _ := builder.Finish()
	return msg
}

func TestDecodeRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		request *Request
	}{
		{
			name: "A Query",
			input: buildQuestionMessage(dnsmessage.Question{
				Name:  dnsmessage.MustNewName("example.com."),
				Type:  dnsmessage.TypeA,
				Class: dnsmessage.ClassINET,
			}),
			request: &Request{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success"},
					QuestionSec: Question{
						Name: "example.com.",
						Type: "A",
					},
				},
				Size: 29,
			},
		},
		{
			name: "AAAA Query",
			input: buildQuestionMessage(dnsmessage.Question{
				Name:  dnsmessage.MustNewName("ipv6.example.com."),
				Type:  dnsmessage.TypeAAAA,
				Class: dnsmessage.ClassINET,
			}),
			request: &Request{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success"},
					QuestionSec: Question{
						Name: "ipv6.example.com.",
						Type: "AAAA",
					},
				},
				Size: 34,
			},
		},
		{
			name: "MX Query",
			input: buildQuestionMessage(dnsmessage.Question{
				Name:  dnsmessage.MustNewName("mail.example.com."),
				Type:  dnsmessage.TypeMX,
				Class: dnsmessage.ClassINET,
			}),
			request: &Request{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success"},
					QuestionSec: Question{
						Name: "mail.example.com.",
						Type: "MX",
					},
				},
				Size: 34,
			},
		},
		{
			name: "PTR Query",
			input: buildQuestionMessage(dnsmessage.Question{
				Name:  dnsmessage.MustNewName("1.1.168.192.in-addr.arpa."),
				Type:  dnsmessage.TypePTR,
				Class: dnsmessage.ClassINET,
			}),
			request: &Request{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success"},
					QuestionSec: Question{
						Name: "1.1.168.192.in-addr.arpa.",
						Type: "PTR",
					},
				},
				Size: 42,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0)
			objs, err := d.Decode(zerocopy.NewBuffer(tt.input), t0)
			assert.NoError(t, err)

			req := objs[0].Obj.(*Request)
			assert.Equal(t, tt.request.Size, req.Size)
			assert.Equal(t, tt.request.Message, req.Message)
		})
	}
}

type resource struct {
	Type   dnsmessage.Type
	Header dnsmessage.ResourceHeader
	Body   any
}

func buildResponse(q dnsmessage.Question, answers []resource, additional ...resource) []byte {
	builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
		ID:       0x01,
		RCode:    dnsmessage.RCodeSuccess,
		Response: true,
	})

	appendResource := func(rs []resource) {
		for _, r := range rs {
			switch r.Type {
			case dnsmessage.TypeA:
				_ = builder.AResource(r.Header, r.Body.(dnsmessage.AResource))
			case dnsmessage.TypeAAAA:
				_ = builder.AAAAResource(r.Header, r.Body.(dnsmessage.AAAAResource))
			case dnsmessage.TypeMX:
				_ = builder.MXResource(r.Header, r.Body.(dnsmessage.MXResource))
			case dnsmessage.TypeTXT:
				_ = builder.TXTResource(r.Header, r.Body.(dnsmessage.TXTResource))
			case dnsmessage.TypePTR:
				_ = builder.PTRResource(r.Header, r.Body.(dnsmessage.PTRResource))
			}
		}
	}

	_ = builder.StartQuestions()
	_ = builder.Question(q)

	_ = builder.StartAnswers()
	appendResource(answers)

	if len(additional) > 0 {
		_ = builder.StartAdditionals()
		appendResource(additional)
	}

	msg, _ := builder.Finish()
	return msg
}

func TestDecodeResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		response *Response
	}{
		{
			name: "A Record Response",
			input: buildResponse(
				dnsmessage.Question{
					Name:  dnsmessage.MustNewName("example.com."),
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
				},
				[]resource{
					{
						Type: dnsmessage.TypeA,
						Header: dnsmessage.ResourceHeader{
							Name:  dnsmessage.MustNewName("example.com."),
							Type:  dnsmessage.TypeA,
							Class: dnsmessage.ClassINET,
							TTL:   300,
						},
						Body: dnsmessage.AResource{A: [4]byte{192, 168, 1, 1}},
					},
				},
			),
			response: &Response{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success", Response: true},
					QuestionSec: Question{
						Name: "example.com.",
						Type: "A",
					},
					AnswerSec: []Answer{{
						Name:   "example.com.",
						Type:   "A",
						Class:  "INET",
						TTL:    300,
						Record: "192.168.1.1",
					}},
				},
				Size: 56,
			},
		},
		{
			name: "CNAME Chain Response",
			input: buildResponse(
				dnsmessage.Question{
					Name:  dnsmessage.MustNewName("www.example.com."),
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
				},
				[]resource{
					{
						Type: dnsmessage.TypeCNAME,
						Header: dnsmessage.ResourceHeader{
							Name:  dnsmessage.MustNewName("www.example.com."),
							Type:  dnsmessage.TypeCNAME,
							Class: dnsmessage.ClassINET,
							TTL:   600,
						},
						Body: dnsmessage.CNAMEResource{
							CNAME: dnsmessage.MustNewName("lb.example.com."),
						},
					},
					{
						Type: dnsmessage.TypeA,
						Header: dnsmessage.ResourceHeader{
							Name:  dnsmessage.MustNewName("lb.example.com."),
							Type:  dnsmessage.TypeA,
							Class: dnsmessage.ClassINET,
							TTL:   300,
						},
						Body: dnsmessage.AResource{A: [4]byte{10, 0, 0, 1}},
					},
				},
			),
			response: &Response{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success", Response: true},
					QuestionSec: Question{
						Name: "www.example.com.",
						Type: "A",
					},
					AnswerSec: []Answer{
						{
							Name:   "lb.example.com.",
							Type:   "A",
							Class:  "INET",
							TTL:    300,
							Record: "10.0.0.1",
						},
					},
				},
				Size: 63,
			},
		},
		{
			name: "MX Record Response",
			input: buildResponse(
				dnsmessage.Question{
					Name:  dnsmessage.MustNewName("example.com."),
					Type:  dnsmessage.TypeMX,
					Class: dnsmessage.ClassINET,
				},
				[]resource{
					{
						Type: dnsmessage.TypeMX,
						Header: dnsmessage.ResourceHeader{
							Name:  dnsmessage.MustNewName("example.com."),
							Type:  dnsmessage.TypeMX,
							Class: dnsmessage.ClassINET,
							TTL:   3600,
						},
						Body: dnsmessage.MXResource{
							Pref: 10,
							MX:   dnsmessage.MustNewName("mail.example.com."),
						},
					},
				},
			),
			response: &Response{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success", Response: true},
					QuestionSec: Question{
						Name: "example.com.",
						Type: "MX",
					},
					AnswerSec: []Answer{{
						Name:   "example.com.",
						Type:   "MX",
						Class:  "INET",
						TTL:    3600,
						Record: "mail.example.com.",
					}},
				},
				Size: 72,
			},
		},
		{
			name: "Empty Response",
			input: buildResponse(
				dnsmessage.Question{
					Name:  dnsmessage.MustNewName("notfound.example.com."),
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
				},
				[]resource{},
			),
			response: &Response{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success", Response: true},
					QuestionSec: Question{
						Name: "notfound.example.com.",
						Type: "A",
					},
				},
				Size: 38,
			},
		},
		{
			name: "AAAA Record Response",
			input: buildResponse(
				dnsmessage.Question{
					Name:  dnsmessage.MustNewName("ipv6.example.com."),
					Type:  dnsmessage.TypeAAAA,
					Class: dnsmessage.ClassINET,
				},
				[]resource{
					{
						Type: dnsmessage.TypeAAAA,
						Header: dnsmessage.ResourceHeader{
							Name:  dnsmessage.MustNewName("ipv6.example.com."),
							Type:  dnsmessage.TypeAAAA,
							Class: dnsmessage.ClassINET,
							TTL:   300,
						},
						Body: dnsmessage.AAAAResource{AAAA: [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x1}},
					},
				},
			),
			response: &Response{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success", Response: true},
					QuestionSec: Question{
						Name: "ipv6.example.com.",
						Type: "AAAA",
					},
					AnswerSec: []Answer{{
						Name:   "ipv6.example.com.",
						Type:   "AAAA",
						Class:  "INET",
						TTL:    300,
						Record: "2001:db8::1",
					}},
				},
				Size: 78,
			},
		},
		{
			name: "TXT Record Response",
			input: buildResponse(
				dnsmessage.Question{
					Name:  dnsmessage.MustNewName("txt.example.com."),
					Type:  dnsmessage.TypeTXT,
					Class: dnsmessage.ClassINET,
				},
				[]resource{{
					Type: dnsmessage.TypeTXT,
					Header: dnsmessage.ResourceHeader{
						Name:  dnsmessage.MustNewName("txt.example.com."),
						Type:  dnsmessage.TypeTXT,
						Class: dnsmessage.ClassINET,
						TTL:   600,
					},
					Body: dnsmessage.TXTResource{TXT: []string{"v=spf1 include:_spf.example.com ~all"}},
				}},
			),
			response: &Response{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success", Response: true},
					QuestionSec: Question{
						Name: "txt.example.com.",
						Type: "TXT",
					},
					AnswerSec: []Answer{{
						Name:   "txt.example.com.",
						Type:   "TXT",
						Class:  "INET",
						TTL:    600,
						Record: "v=spf1 include:_spf.example.com ~all",
					}},
				},
				Size: 97,
			},
		},
		{
			name: "Multiple A Records",
			input: buildResponse(
				dnsmessage.Question{
					Name:  dnsmessage.MustNewName("multi.example.com."),
					Type:  dnsmessage.TypeA,
					Class: dnsmessage.ClassINET,
				},
				[]resource{
					{
						Type: dnsmessage.TypeA,
						Header: dnsmessage.ResourceHeader{
							Name:  dnsmessage.MustNewName("multi.example.com."),
							Type:  dnsmessage.TypeA,
							Class: dnsmessage.ClassINET,
							TTL:   300,
						},
						Body: dnsmessage.AResource{A: [4]byte{192, 168, 1, 1}},
					},
					{
						Type: dnsmessage.TypeA,
						Header: dnsmessage.ResourceHeader{
							Name:  dnsmessage.MustNewName("multi.example.com."),
							Type:  dnsmessage.TypeA,
							Class: dnsmessage.ClassINET,
							TTL:   300,
						},
						Body: dnsmessage.AResource{A: [4]byte{192, 168, 1, 2}},
					},
				},
			),
			response: &Response{
				Message: Message{
					Header: Header{ID: 0x01, OpCode: "Query", Status: "Success", Response: true},
					QuestionSec: Question{
						Name: "multi.example.com.",
						Type: "A",
					},
					AnswerSec: []Answer{
						{
							Name:   "multi.example.com.",
							Type:   "A",
							Class:  "INET",
							TTL:    300,
							Record: "192.168.1.1",
						},
						{
							Name:   "multi.example.com.",
							Type:   "A",
							Class:  "INET",
							TTL:    300,
							Record: "192.168.1.2",
						},
					},
				},
				Size: 101,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0)
			objs, err := d.Decode(zerocopy.NewBuffer(tt.input), t0)
			assert.NoError(t, err)

			resp := objs[0].Obj.(*Response)
			assert.Equal(t, tt.response.Size, resp.Size)
			assert.Equal(t, tt.response.Message, resp.Message)
		})
	}
}

func TestDecodeFailed(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "Invalid Header",
			input: []byte{0x01},
		},
		{
			name: "Truncated Question",
			input: []byte{
				0x00, 0x01, // ID=1
				0x00, 0x00, // Flags
				0x00, 0x01, // QDCOUNT=1
				0x00, 0x00, // ANCOUNT=0
				0x00, 0x00, // NSCOUNT=0
				0x00, 0x00, // ARCOUNT=0
				0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e', // 问题部分被截断
			},
		},
		{
			name: "Invalid Domain Compression",
			input: []byte{
				0x00, 0x01, // ID=1
				0x80, 0x00, // Flags: QR=1
				0x00, 0x01, // QDCOUNT=1
				0x00, 0x00, 0x00, 0x00, // 其他计数
				0x03, 'w', 'w', 'w',
				0xc0, 0x20, // 无效的压缩指针（指向偏移 32）
				0x00, 0x01, 0x00, 0x01,
			},
		},
		{
			name: "Invalid Opcode",
			input: []byte{
				0x00, 0x01, // ID=1
				0x78, 0x00, // Opcode=15 (保留值)
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
		{
			name: "Invalid Rcode",
			input: []byte{
				0x00, 0x01, // ID=1
				0x00, 0x0f, // Rcode=15 (保留值)
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
	}

	var st socket.Tuple
	var t0 time.Time
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDecoder(st, 0)
			objs, _ := d.Decode(zerocopy.NewBuffer(tt.input), t0)
			assert.Empty(t, objs)
		})
	}
}
