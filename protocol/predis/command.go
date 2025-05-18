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

package predis

import (
	"bytes"
	_ "embed"
	"strings"
)

//go:embed command.list
var commandContent string

var (
	commands    map[string]struct{}
	subCommands map[string]map[string]struct{}
)

func init() {
	commands = make(map[string]struct{})
	subCommands = make(map[string]map[string]struct{})

	for _, s := range strings.Split(commandContent, "\n") {
		if s == "" {
			continue
		}

		fields := strings.Fields(s)
		cmd := fields[0]
		commands[cmd] = struct{}{}
		if len(fields) > 1 {
			_, ok := subCommands[cmd]
			if !ok {
				subCommands[cmd] = make(map[string]struct{})
			}
			subCommands[cmd][fields[1]] = struct{}{}
		}
	}
}

const maxCommandLen = 64

// normalizeCommand 标准化命令 如果 cmd 不合法则返回空字符串
func normalizeCommand(b []byte) string {
	l := maxCommandLen
	if l > len(b) {
		l = len(b)
	}

	fields := bytes.Fields(bytes.ToUpper(b[:l]))
	if len(fields) == 0 {
		return ""
	}

	cmd := string(fields[0])
	if _, ok := commands[cmd]; !ok {
		return ""
	}
	return cmd
}

// normalizeSubCommand 标准化子命令 如果 subcmd 不合法则返回空字符串
func normalizeSubCommand(cmd string, b []byte) string {
	l := maxCommandLen
	if l > len(b) {
		l = len(b)
	}

	fields := bytes.Fields(bytes.ToUpper(b[:l]))
	if len(fields) == 0 {
		return ""
	}

	subs, ok := subCommands[cmd]
	if !ok {
		return ""
	}

	sub := string(fields[0])
	if _, ok = subs[sub]; !ok {
		return ""
	}
	return sub
}
