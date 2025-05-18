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

package labels

import (
	"github.com/cespare/xxhash/v2"
	"github.com/valyala/bytebufferpool"
)

type Label struct {
	Name  string
	Value string
}

type Labels []Label

func (ls Labels) Len() int {
	return len(ls)
}

func (ls Labels) Less(i, j int) bool {
	return ls[i].Name < ls[j].Name
}

func (ls Labels) Swap(i, j int) {
	ls[i], ls[j] = ls[j], ls[i]
}

var seps = []byte{'\xff'}

// Hash returns a hash value for the label set.
func (ls Labels) Hash() uint64 {
	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)

	for _, v := range ls {
		buf.WriteString(v.Name)
		buf.Write(seps)
		buf.WriteString(v.Value)
		buf.Write(seps)
	}
	h := xxhash.Sum64(buf.Bytes())
	return h
}
