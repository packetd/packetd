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
	_ "embed"
	"strconv"
	"strings"
)

//go:embed codemsg.list
var codeMsgContent string

var codeMessages = map[int32]string{}

func init() {
	codeMessages = make(map[int32]string)
	for _, s := range strings.Split(codeMsgContent, "\n") {
		if s == "" {
			continue
		}
		fields := strings.Split(s, ":")
		if len(fields) != 2 {
			continue
		}

		code, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			continue
		}
		msg := strings.TrimSpace(fields[1])
		codeMessages[int32(code)] = msg
	}
}
