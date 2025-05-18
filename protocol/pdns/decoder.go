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
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol"
	"github.com/packetd/packetd/protocol/role"
)

const (
	PROTO = "DNS"
)

// Message 报文布局
//
// rfc: https://www.ietf.org/rfc/rfc1035.txt 4.1. Format
//
// +---------------------+
// |       Header        | → Fixed 12 bytes
// +---------------------+
// |      Question       | → Query details (name, type, class)
// +---------------------+
// |      Answer(s)      | → Resource Records (RRs)
// +---------------------+
// |   Authority (RRs)   | → Authoritative servers
// +---------------------+
// |  Additional (RRs)   | → Additional data (e.g., IPv6)
// +---------------------+
//
// Request / Response 可以通过 Header 字段来区分
type Message struct {
	Header        Header
	QuestionSec   Question
	AnswerSec     []Answer     `json:",omitempty"`
	AuthoritySec  []Authority  `json:",omitempty"`
	AdditionalSec []Additional `json:",omitempty"`
}

type decoder struct {
	st         socket.TupleRaw
	t0         time.Time
	p          dnsmessage.Parser
	drainBytes int
	msg        *Message
}

func NewDecoder(st socket.Tuple, _ socket.Port) protocol.Decoder {
	return &decoder{
		st:  st.ToRaw(),
		msg: &Message{},
	}
}

// Decode 从 zerocopy.Reader 中不断解析来自 Request / Response 的数据 并判断是否能构建成 RoundTrip
//
// decoder 仅解析基于 UDP 的 DNS 报文 使用了 dnsmessage.Parser 库
// 解析并不会拼接多个数据包 单次 Decode 直接决定了能否归档为 *role.Object
//
// 请求示例:
// +----------------------+                     +----------------------+
// |      DNS Client      |                     |      DNS Server      |
// +----------------------+                     +----------------------+
// |                      |  --[Query UDP 53]→  |                      |
// | Header:              |                     |                      |
// |  ID=0x1234, QR=0     |                     |                      |
// |  QDCOUNT=1           |                     |                      |
// | Question:            |                     |                      |
// |  Name=www.example.com|                     |                      |
// |  Type=A (0x0001)     |                     |                      |
// |  Class=IN (0x0001)   |                     |                      |
// +----------------------+                     +----------------------+
// |                      | ←[Response UDP 53]- | ←[Process Query]     |
// |                      |                     | Generate Answer:     |
// |                      |                     | Name=www.example.com |
// |                      |                     | Type=A, TTL=300      |
// |                      |                     | Data=93.184.216.34   |
// +----------------------+                     +----------------------+
//
// DNS 可能返回一个或者多个 Record 分为 4 个 Section 需要按序按序
// - Question Section (Q)
// - Answer Section (A)
// - Authority Section (AA)
// - Additional Section (AD)
//
// 因为单数据包即可完成请求 不再区分 Reqeust / Response Time 直接按 t0 归档即可 也无须重置
func (d *decoder) Decode(r zerocopy.Reader, t time.Time) ([]*role.Object, error) {
	d.t0 = t

	b, err := r.Read(common.ReadWriteBlockSize)
	if err != nil {
		return nil, nil
	}

	obj, err := d.decode(b)
	if err != nil {
		return nil, err
	}

	if obj == nil {
		return nil, nil
	}
	return []*role.Object{obj}, nil
}

// Free 释放持有的资源
func (d *decoder) Free() {
	d.drainBytes = 0
}

// archive 归档请求
func (d *decoder) archive() *role.Object {
	if !d.msg.Header.Response {
		obj := role.NewRequestObject(&Request{
			Host:    d.st.SrcIP,
			Port:    d.st.SrcPort,
			Proto:   PROTO,
			Size:    d.drainBytes,
			Time:    d.t0,
			Message: *d.msg,
		})
		return obj
	}

	obj := role.NewResponseObject(&Response{
		Host:    d.st.SrcIP,
		Port:    d.st.SrcPort,
		Proto:   PROTO,
		Size:    d.drainBytes,
		Time:    d.t0,
		Message: *d.msg,
	})
	return obj
}

// decode 解析入口 按序解析各个 section
func (d *decoder) decode(b []byte) (*role.Object, error) {
	if err := d.decodeHeader(b); err != nil {
		return nil, err
	}

	states := []func() error{
		d.decodeQuestion,
		d.decodeAnswer,
		d.decodeAuthority,
		d.decodeAdditional,
	}
	for _, f := range states {
		if err := f(); err != nil {
			return nil, err
		}
	}
	return d.archive(), nil
}

// Header DNS Header 字段
type Header struct {
	ID       uint16
	OpCode   string
	Status   string
	Response bool
}

// decodeHeader 解析 DNS Header 报文布局如下
//
// 0  1  2  3  4  5  6  7  8  9  A  B  C  D  E  F
// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
// |                     ID (2 bytes)               | → Transaction ID
// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
// |QR| Opcode |AA|TC|RD|RA| Z    |   RCODE         | → Flags & status
// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
// |          QDCOUNT (2 bytes)                    | → # of questions
// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
// |          ANCOUNT (2 bytes)                    | → # of answers
// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
// |          NSCOUNT (2 bytes)                    | → # of authority RRs
// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
// |          ARCOUNT (2 bytes)                    | → # of additional RRs
// +--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
//
// Flags:
//
// * ID: 请求与响应匹配的标识符
// * QR: 0=查询 1=响应
// * Opcode: 操作类型（0=标准查询）
// * AA: 权威回答（响应中有效）
// * TC: 截断标志（响应过长时置 1）
// * RD/RA: 递归查询请求/可用
// * RCODE: 响应错误码（0=无错误 3=NXDOMAIN）
//
// 解析的第一步 启动解析器
func (d *decoder) decodeHeader(b []byte) error {
	header, err := d.p.Start(b)
	if err != nil {
		return err
	}

	d.drainBytes += len(b) // 记录请求字节数
	d.msg.Header = Header{
		ID:       header.ID,
		OpCode:   matchOpCodeName(header.OpCode),
		Status:   matchRcodeName(header.RCode),
		Response: header.Response,
	}
	return nil
}

// Question DNS Question 字段
type Question struct {
	Name string
	Type string
}

// decodeQuestion 解析 DNS Question 报文布局如下
//
// +---------------------+
// |      Name           | → Domain name (variable length)
// +---------------------+
// |    Type (2 bytes)   | → 1=A, 28=AAAA, 5=CNAME, 15=MX, etc.
// +---------------------+
// |    Class (2 bytes)  | → 1=IN (Internet)
// +---------------------+
//
// 域名编码示例 www.example.com
// +---+---+---+---+---+---+---+---+---+---+---+---+---+
// | 3 | w | w | w | 7 | e | x | a | m | p | l | e | 0 |
// +---+---+---+---+---+---+---+---+---+---+---+---+---+
// ↑           ↑           ↑          ↑
// 标签长度=3  "www"       标签长度=7  "example"       终止符
//
// 仅解析一个 questions
func (d *decoder) decodeQuestion() error {
	for {
		q, err := d.p.Question()
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}

		d.msg.QuestionSec.Name = q.Name.String()
		d.msg.QuestionSec.Type = matchTypeName(q.Type)
		if err := d.p.SkipAllQuestions(); err != nil {
			return err
		}
		break
	}
	return nil
}

// decodeResourceRecord 解析 DNS ResourceRecord 报文布局如下
//
// +---------------------+
// |      Name           | → Domain name (may use pointers, e.g., 0xC00C)
// +---------------------+
// |    Type (2 bytes)   |
// +---------------------+
// |    Class (2 bytes)  |
// +---------------------+
// |    TTL (4 bytes)    | → Time-to-live (seconds)
// +---------------------+
// |  Data Len (2 bytes) | → Length of RDATA
// +---------------------+
// |       RDATA         | → Variable-length data (IP, CNAME, etc.)
// +---------------------+
func (d *decoder) decodeResourceRecord(t dnsmessage.Type) (string, bool, error) {
	var unknown bool
	var s string
	switch t {
	case dnsmessage.TypeA:
		r, err := d.p.AResource()
		if err != nil {
			return "", unknown, err
		}
		s = ipString(r.A[:])

	case dnsmessage.TypeAAAA:
		r, err := d.p.AAAAResource()
		if err != nil {
			return "", unknown, err
		}
		s = ipString(r.AAAA[:])

	case dnsmessage.TypeCNAME:
		r, err := d.p.CNAMEResource()
		if err != nil {
			return "", unknown, err
		}
		s = r.CNAME.String()

	case dnsmessage.TypeMX:
		r, err := d.p.MXResource()
		if err != nil {
			return "", unknown, err
		}
		s = r.MX.String()

	case dnsmessage.TypePTR:
		r, err := d.p.PTRResource()
		if err != nil {
			return "", unknown, err
		}
		s = r.PTR.String()

	case dnsmessage.TypeSRV:
		r, err := d.p.SRVResource()
		if err != nil {
			return "", unknown, err
		}
		s = fmt.Sprintf("%s:%d/W:%d/P:%d", r.Target.String(), r.Port, r.Weight, r.Priority)

	case dnsmessage.TypeSOA:
		r, err := d.p.SOAResource()
		if err != nil {
			return "", unknown, err
		}
		s = fmt.Sprintf("%s/%s/T:%d/E:%d", r.NS.String(), r.MBox.String(), r.MinTTL, r.Expire)

	case dnsmessage.TypeTXT:
		r, err := d.p.TXTResource()
		if err != nil {
			return "", unknown, err
		}
		s = strings.Join(r.TXT, "/")

	case dnsmessage.TypeNS:
		r, err := d.p.NSResource()
		if err != nil {
			return "", unknown, err
		}
		s = r.NS.String()

	default:
		unknown = true
		_, _ = d.p.UnknownResource()
	}

	return s, unknown, nil
}

// 常见 DNS 记录内容如下
//
// +------------+------+-------------------------------+----------------------------+
// | Type       | Code | Description                   | Example Data               |
// +============+======+===============================+============================+
// | A          | 1    | IPv4 Address                  | 192.0.2.1                  |
// +------------+------+-------------------------------+----------------------------+
// | AAAA       | 28   | IPv6 Address                  | 2001:db8::1                |
// +------------+------+-------------------------------+----------------------------+
// | CNAME      | 5    | Canonical Name (Alias)        | cdn.example.com            |
// +------------+------+-------------------------------+----------------------------+
// | MX         | 15   | Mail Server (Priority+Domain) | 10 mail.example.com        |
// +------------+------+-------------------------------+----------------------------+
// | NS         | 2    | Authoritative Name Server     | ns1.example.com            |
// +------------+------+-------------------------------+----------------------------+
// | PTR        | 12   | Reverse DNS (IP → Domain)     | www.example.com            |
// +------------+------+-------------------------------+----------------------------+
// | SOA        | 6    | Zone Authority Metadata       | ns1.example.com.           |
// |            |      |                               | admin.example.com.         |
// |            |      |                               | 2023082801 7200 3600 ...   |
// +------------+------+-------------------------------+----------------------------+
// | TXT        | 16   | Text Data                     | "v=spf1 mx -all"           |
// |            |      | (SPF/DKIM/Verification)       |                            |
// +------------+------+-------------------------------+----------------------------+
// | SRV        | 33   | Service Location Protocol     | 10 5 5060 sip.example.com  |
// +------------+------+-------------------------------+----------------------------+
// | CAA        | 257  | SSL Certificate Authorization | 0 issue "letsencrypt.org"  |
// +------------+------+-------------------------------+----------------------------+

// Answer DNS Answer 字段
type Answer struct {
	Name   string
	Type   string
	TTL    uint32
	Class  string
	Record string
}

// decodeAnswer 解析 DNS Answer Section
//
// 常见的有 A/CNAME/AAAA/MX 等回答记录 在 ResourceRecord 中进行逐个解析
func (d *decoder) decodeAnswer() error {
	for {
		h, err := d.p.AnswerHeader()
		if err != nil || errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}

		answer := Answer{
			Name:  h.Name.String(),
			TTL:   h.TTL,
			Class: matchClassName(h.Class),
			Type:  matchTypeName(h.Type),
		}

		record, unknown, err := d.decodeResourceRecord(h.Type)
		if err != nil {
			return err
		}
		if !unknown {
			answer.Record = record
			d.msg.AnswerSec = append(d.msg.AnswerSec, answer)
		}
	}
	return nil
}

// Authority DNS Authority 字段
type Authority struct {
	Name   string
	Type   string
	Class  string
	Record string
}

// decodeAuthority 解析 DNS Authority Section
//
// 常见的有 NS/SOA 等回答记录 在 ResourceRecord 中进行逐个解析
func (d *decoder) decodeAuthority() error {
	for {
		h, err := d.p.AuthorityHeader()
		if err != nil || errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}

		record, unknown, err := d.decodeResourceRecord(h.Type)
		if err != nil {
			return err
		}
		if !unknown {
			d.msg.AuthoritySec = append(d.msg.AuthoritySec, Authority{
				Name:   h.Name.String(),
				Type:   matchTypeName(h.Type),
				Class:  matchClassName(h.Class),
				Record: record,
			})
		}
	}

	return nil
}

// Additional DNS Additional 字段
type Additional struct {
	Name   string
	Type   string
	Class  string
	Record string
}

// decodeAdditional 解析 DNS Additional Section
//
// Additional 提供与查询相关的额外信息 避免客户端发起额外 DNS 请求
// 常见的有 A/AAAA/TXT 等回答记录
func (d *decoder) decodeAdditional() error {
	for {
		h, err := d.p.Additional()
		if err != nil || errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}

		additional := Additional{
			Name:  h.Header.Name.String(),
			Class: matchClassName(h.Header.Class),
		}

		switch r := h.Body.(type) {
		case *dnsmessage.AResource:
			additional.Record = ipString(r.A[:])
			additional.Type = matchTypeName(dnsmessage.TypeA)

		case *dnsmessage.AAAAResource:
			additional.Record = ipString(r.AAAA[:])
			additional.Type = matchTypeName(dnsmessage.TypeAAAA)

		case *dnsmessage.CNAMEResource:
			additional.Record = r.CNAME.String()
			additional.Type = matchTypeName(dnsmessage.TypeCNAME)

		case *dnsmessage.NSResource:
			additional.Record = r.NS.String()
			additional.Type = matchTypeName(dnsmessage.TypeNS)

		case *dnsmessage.MXResource:
			additional.Record = r.MX.String()
			additional.Type = matchTypeName(dnsmessage.TypeMX)

		case *dnsmessage.PTRResource:
			additional.Record = r.PTR.String()
			additional.Type = matchTypeName(dnsmessage.TypePTR)

		case *dnsmessage.SRVResource:
			additional.Record = fmt.Sprintf("%s:%d/W:%d/P:%d", r.Target.String(), r.Port, r.Weight, r.Priority)
			additional.Type = matchTypeName(dnsmessage.TypeSRV)

		case *dnsmessage.SOAResource:
			additional.Record = fmt.Sprintf("%s/%s/T:%d/E:%d", r.NS.String(), r.MBox.String(), r.MinTTL, r.Expire)
			additional.Type = matchTypeName(dnsmessage.TypeSOA)

		case *dnsmessage.TXTResource:
			additional.Record = strings.Join(r.TXT, "/")
			additional.Type = matchTypeName(dnsmessage.TypeTXT)
		}

		if additional.Type != "" {
			d.msg.AdditionalSec = append(d.msg.AdditionalSec, additional)
		}
	}

	return nil
}

func ipString(b []byte) string {
	return net.IP(b).String()
}
