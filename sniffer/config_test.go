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

package sniffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompileBPFFilter(t *testing.T) {
	tests := []struct {
		name  string
		rules []ProtoRule
		want  string
	}{
		{
			name: "Single protocol rule",
			rules: []ProtoRule{
				{
					Protocol: "http",
					Host:     "example.com",
					Ports:    []uint16{80},
				},
			},
			want: "(tcp and host example.com and port 80)",
		},
		{
			name: "Nil ports",
			rules: []ProtoRule{
				{
					Protocol: "http",
					Host:     "example.com",
					Ports:    []uint16{80},
				},
				{
					Protocol: "redis",
					Host:     "example.com",
					Ports:    nil,
				},
			},
			want: "(tcp and host example.com and port 80)",
		},
		{
			name: "Multiple protocol rules",
			rules: []ProtoRule{
				{
					Protocol: "http",
					Host:     "example.com",
					Ports:    []uint16{80, 8080},
				},
				{
					Protocol: "dns",
					Ports:    []uint16{53},
				},
			},
			want: "(tcp and host example.com and ( port 80 or port 8080 ) ) or (udp and port 53)",
		},
		{
			name: "Unsupported protocol",
			rules: []ProtoRule{
				{
					Protocol: "unknown",
				},
			},
			want: "",
		},
		{
			name: "Without host and port",
			rules: []ProtoRule{
				{
					Protocol: "http",
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := Protocols{Rules: tt.rules}
			got, _ := ps.CompileBPFFilter()
			assert.Equal(t, tt.want, got)
		})
	}
}
