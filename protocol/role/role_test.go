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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSingleMatcher(t *testing.T) {
	tests := []struct {
		input []Role
		want  int
	}{
		{
			input: []Role{Request, Request},
			want:  0,
		},
		{
			input: []Role{Response, Request},
			want:  0,
		},
		{
			input: []Role{Request, Response},
			want:  1,
		},
		{
			input: []Role{Response, Response, Request},
			want:  0,
		},
		{
			input: []Role{Response, Request, Response},
			want:  1,
		},
		{
			input: []Role{Request, Request, Response},
			want:  0,
		},
		{
			input: []Role{Request, Request, Response, Response},
			want:  0,
		},
		{
			input: []Role{Request, Request, Response, Response, Response, Request, Response},
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			matcher := NewSingleMatcher()
			var count int
			for _, role := range tt.input {
				if matcher.Match(&Object{Role: role}) != nil {
					count++
				}
			}
			assert.Equal(t, tt.want, count)
		})
	}
}

func TestListMatcher(t *testing.T) {
	tests := []struct {
		objs []*Object
		want int
	}{
		{
			objs: []*Object{
				NewRequestObject(1),
				NewResponseObject(1),
			},
			want: 1,
		},
		{
			objs: []*Object{
				NewResponseObject(1),
				NewRequestObject(1),
				NewResponseObject(2),
			},
			want: 0,
		},
		{
			objs: []*Object{
				NewRequestObject(1),
				NewResponseObject(1),
				NewResponseObject(1),
			},
			want: 1,
		},
		{
			objs: []*Object{
				NewRequestObject(1),
				NewRequestObject(2),
				NewResponseObject(3), // 不匹配
				NewResponseObject(2),
				NewResponseObject(1),
			},
			want: 2,
		},
		{
			objs: []*Object{
				NewRequestObject(1),
				NewRequestObject(2),
				NewRequestObject(3),
				NewRequestObject(4), // 触发淘汰
				NewResponseObject(2),
				NewResponseObject(4),
			},
			want: 2,
		},
		{
			objs: []*Object{
				NewRequestObject(1),
				NewResponseObject(1),
				NewRequestObject(2),
				NewResponseObject(2),
				NewRequestObject(3),
				NewResponseObject(3),
			},
			want: 3,
		},
		{
			objs: []*Object{
				NewRequestObject(1),
				NewRequestObject(2),
				NewRequestObject(3),
				NewResponseObject(3),
				NewResponseObject(2),
				NewResponseObject(1),
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			matcher := NewListMatcher(3, func(req, rsp *Object) bool {
				return req.Obj.(int) == rsp.Obj.(int)
			})

			var count int
			for _, obj := range tt.objs {
				if matcher.Match(obj) != nil {
					count++
				}
			}
			assert.Equal(t, tt.want, count)
		})
	}
}

func TestFuzzyMatcher(t *testing.T) {
	tests := []struct {
		objs []*Object
		want int
	}{
		{
			objs: []*Object{
				NewResponseObject(1),
				NewRequestObject(1),
			},
			want: 1,
		},
		{
			objs: []*Object{
				NewResponseObject(1),
				NewResponseObject(2),
				NewRequestObject(1),
			},
			want: 1,
		},
		{
			objs: []*Object{
				NewResponseObject(2),
				NewResponseObject(1),
				NewResponseObject(3), // 不匹配
				NewRequestObject(1),
				NewRequestObject(2),
			},
			want: 2,
		},
		{
			objs: []*Object{
				NewRequestObject(1),
				NewRequestObject(3),
				NewResponseObject(2),
				NewRequestObject(4), // 触发淘汰
				NewRequestObject(2),
				NewResponseObject(4),
			},
			want: 2,
		},
		{
			objs: []*Object{
				NewResponseObject(1),
				NewRequestObject(1),
				NewRequestObject(2),
				NewResponseObject(2),
				NewResponseObject(3),
				NewRequestObject(3),
			},
			want: 3,
		},
		{
			objs: []*Object{
				NewResponseObject(3),
				NewResponseObject(2),
				NewResponseObject(1),
				NewRequestObject(3),
				NewRequestObject(2),
				NewRequestObject(1),
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			matcher := NewFuzzyMatcher(4, func(req, rsp *Object) bool {
				return req.Obj.(int) == rsp.Obj.(int)
			})

			var count int
			for _, obj := range tt.objs {
				if matcher.Match(obj) != nil {
					count++
				}
			}
			assert.Equal(t, tt.want, count)
		})
	}
}
