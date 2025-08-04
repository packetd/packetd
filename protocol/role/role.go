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

package role

import (
	"container/list"
)

// Role 代表着一个网络来回中通信双方的角色
//
// TCP Connection 是全双工没有方向的 但是应用层是有角色区分的
// * Client <-> Server
type Role string

const (
	Request  = "Request"
	Response = "Response"
)

// Object 代表通信一方归档的对象
//
// Obj 为各种协议的 *Request 或者 *Response 对象
type Object struct {
	Role Role
	Obj  any
}

// NewRequestObject 创建一个 Request 对象
func NewRequestObject(obj any) *Object {
	return &Object{
		Role: Request,
		Obj:  obj,
	}
}

// NewResponseObject 创建一个 Response 对象
func NewResponseObject(obj any) *Object {
	return &Object{
		Role: Response,
		Obj:  obj,
	}
}

// Pair 代表一个完整的网络请求中的 *Request 和 *Response 组合
type Pair struct {
	Request  *Object
	Response *Object
}

// Matcher 请求匹配器
//
// Decoder 归档之后会生成一个 *Object 对象 Matcher 则负责结对 *Request 和 *Response
type Matcher interface {
	Match(o *Object) *Pair
}

// SingleMatcher 单对象匹配器
//
// 对于同个链接请求是 Request <-> Response 单次来回的使用此 Matcher
type SingleMatcher struct {
	o *Object
}

func NewSingleMatcher() Matcher {
	return &SingleMatcher{}
}

func (m *SingleMatcher) Match(o *Object) *Pair {
	if m.o == nil {
		if o.Role == Response {
			return nil
		}
		m.o = o // Request
		return nil
	}

	if m.o.Role == Request {
		if o.Role == Request {
			m.o = nil
			return nil
		}

		pair := &Pair{
			Request:  m.o,
			Response: o,
		}
		m.o = nil
		return pair
	}
	return nil
}

// ListMatcher 列表匹配器
//
// 对于同个链接请求是同时支持多次 Request <-> Response 的使用此 Matcher
// 如 HTTP2、GPRC 中支持多路复用的情况
type ListMatcher struct {
	l         *list.List
	size      int // 超限驱逐
	matchFunc func(a, b *Object) bool
}

func NewListMatcher(size int, matchFunc func(req, rsp *Object) bool) Matcher {
	return &ListMatcher{
		size:      size,
		matchFunc: matchFunc,
		l:         list.New(),
	}
}

func (m *ListMatcher) Match(o *Object) *Pair {
	if o.Role == Request {
		if m.l.Len() >= m.size {
			m.l.Remove(m.l.Front())
		}
		m.l.PushBack(o)
		return nil
	}

	for e := m.l.Front(); e != nil; e = e.Next() {
		if m.matchFunc(e.Value.(*Object), o) {
			pair := &Pair{
				Request:  e.Value.(*Object),
				Response: o,
			}
			m.l.Remove(e)
			return pair
		}
	}
	return nil
}

// FuzzyMatcher 模糊匹配器
//
// FuzzyMatcher 不关心是 Request / Response 哪个先到 仅保证最大程度完成匹配
type FuzzyMatcher struct {
	l         *list.List
	size      int // 超限驱逐
	matchFunc func(a, b *Object) bool
}

func NewFuzzyMatcher(size int, matchFunc func(o1, o2 *Object) bool) Matcher {
	return &FuzzyMatcher{
		size:      size,
		matchFunc: matchFunc,
		l:         list.New(),
	}
}

func (m *FuzzyMatcher) Match(o *Object) *Pair {
	if m.l.Len() >= m.size {
		m.l.Remove(m.l.Front())
	}

	switch o.Role {
	case Request:
		for e := m.l.Front(); e != nil; e = e.Next() {
			if e.Value.(*Object).Role == Request {
				continue
			}

			if m.matchFunc(o, e.Value.(*Object)) {
				pair := &Pair{
					Request:  o,
					Response: e.Value.(*Object),
				}
				m.l.Remove(e)
				return pair
			}
		}
		m.l.PushBack(o)

	case Response:
		for e := m.l.Front(); e != nil; e = e.Next() {
			if e.Value.(*Object).Role == Response {
				continue
			}

			if m.matchFunc(e.Value.(*Object), o) {
				pair := &Pair{
					Request:  e.Value.(*Object),
					Response: o,
				}
				m.l.Remove(e)
				return pair
			}
		}
		m.l.PushBack(o)
	}

	return nil
}
