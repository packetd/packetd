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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeCommand(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "Empty input",
			input: []byte(""),
			want:  "",
		},
		{
			name:  "Whitespace only",
			input: []byte("    \t\n"),
			want:  "",
		},
		{
			name:  "Invalid command",
			input: []byte("INVALID_COMMAND"),
			want:  "",
		},
		{
			name:  "GET command",
			input: []byte("get"),
			want:  "GET",
		},
		{
			name:  "SET command with args",
			input: []byte("SET mykey value"),
			want:  "SET",
		},
		{
			name:  "Command exceeds max length",
			input: []byte("SET " + string(make([]byte, maxCommandLen))),
			want:  "SET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeCommand(tt.input))
		})
	}
}

func TestNormalizeSubCommand(t *testing.T) {
	tests := []struct {
		name  string
		cmd   string
		input []byte
		want  string
	}{
		{
			name:  "Client kill with args",
			input: []byte("KILL addr 127.0.0.1 port 6379"),
			cmd:   "CLIENT",
			want:  "KILL",
		},
		{
			name:  "Client list subcommand",
			input: []byte("list"),
			cmd:   "CLIENT",
			want:  "LIST",
		},
		{
			name:  "Subcommand exceeds max length",
			input: []byte("INFO " + string(make([]byte, maxCommandLen))),
			cmd:   "CLIENT",
			want:  "INFO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeSubCommand(tt.cmd, tt.input))
		})
	}
}
