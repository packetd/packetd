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
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common/socket"
)

type Config struct {
	Ifaces    string    `config:"ifaces"`
	Engine    string    `config:"engine"`
	IPv4Only  bool      `config:"ipv4Only"`
	Protocols Protocols `config:"protocols"`
}

type ProtoRule struct {
	Name     string `config:"name"`
	Protocol string `config:"protocol"`
	Host     string `config:"host"`
	Port     uint16 `config:"port"`
}

func (r ProtoRule) compileBPFFilter(layer4 string) string {
	var buf strings.Builder
	buf.WriteString("(")
	buf.WriteString(layer4)

	if r.Host != "" {
		buf.WriteString(" and host ")
		buf.WriteString(r.Host)
	}

	if r.Port > 0 {
		buf.WriteString(" and port ")
		buf.WriteString(strconv.Itoa(int(r.Port)))
	}

	buf.WriteString(")")
	return buf.String()
}

type Protocols struct {
	Rules []ProtoRule `config:"rules"`
}

func (ps Protocols) CompileBPFFilter() (string, error) {
	var filters []string
	for _, p := range ps.Rules {
		l4, ok := socket.L7ProtoBased(socket.L7Proto(p.Protocol))
		if !ok {
			return "", errors.Errorf("unsupported protocol (%s)", p.Protocol)
		}
		filters = append(filters, p.compileBPFFilter(string(l4)))
	}
	return strings.Join(filters, " or "), nil
}

func (ps Protocols) L7Ports() []socket.L7Port {
	var ports []socket.L7Port
	for _, proto := range ps.Rules {
		ports = append(ports, socket.L7Port{
			Port:  socket.Port(proto.Port),
			Proto: socket.L7Proto(proto.Protocol),
		})
	}
	return ports
}
