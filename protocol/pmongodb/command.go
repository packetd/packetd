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
	"strings"
)

//go:embed command.list
var commandContent string

var (
	commands                map[string]struct{}
	commandsWithCollections map[string]struct{}
)

func init() {
	commands = make(map[string]struct{})
	commandsWithCollections = make(map[string]struct{})
	for _, s := range strings.Split(commandContent, "\n") {
		if s == "" {
			continue
		}

		// 末尾带分号的表示 cmd value 为 collection name
		if strings.HasSuffix(s, ";") {
			s = s[:len(s)-1]
			commandsWithCollections[s] = struct{}{}
		}
		commands[s] = struct{}{}
	}
}

func isCommand(s string) bool {
	_, ok := commands[s]
	return ok
}

func isCommandWithCollection(s string) bool {
	_, ok := commandsWithCollections[s]
	return ok
}

// opcode MongoDB OpCode
// https://www.mongodb.com/docs/manual/reference/mongodb-wire-protocol/
type opcode int32

const (
	opcodeCompressed opcode = 2012
	opcodeMsg        opcode = 2013
	opcodeReply      opcode = 1
	opcodeUpdate     opcode = 2001
	opcodeInsert     opcode = 2002
	opcodeReserved   opcode = 2003
	opcodeQuery      opcode = 2004
	opcodeGetMore    opcode = 2005
	opcodeDelete     opcode = 2006
	opcodeKillCursor opcode = 2007
)

var opcodes = map[opcode]string{
	opcodeCompressed: "COMPRESSED",
	opcodeMsg:        "MSG",
	opcodeReply:      "REPLY",
	opcodeUpdate:     "UPDATE",
	opcodeInsert:     "INSERT",
	opcodeReserved:   "RESERVED",
	opcodeQuery:      "QUERY",
	opcodeGetMore:    "GET_MORE",
	opcodeDelete:     "DELETE",
	opcodeKillCursor: "KILL_CURSORS",
}
