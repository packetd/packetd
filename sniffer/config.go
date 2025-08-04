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
	// File 指定是否从文件中加载网络包 与监听网卡选项互斥
	File string `config:"file"`

	// Ifaces 指定监听的网卡 与 tcpdump 的 -i 参数一致
	Ifaces string `config:"ifaces"`

	// Engine 指定监听引擎 目前仅支持 pcap
	Engine string `config:"engine"`

	// IPVersion 指定监听 ipv4/ipv6 可选值为
	// - v4
	// - v6
	// 空值或其他非法值均代表同时监听两者
	IPVersion string `config:"ipVersion"`

	// Protocols 声明解析协议以及端口 使用列表允许同时指定多个协议
	// - name: 规则名称
	// - protocol: 协议名称
	// - ports: 端口号列表
	Protocols Protocols `config:"protocols"`

	// NoPromisc 是否关闭 promiscuous 模式
	NoPromisc bool `config:"noPromisc"`

	// BlockNum 缓冲区 block 数量（仅 Linux 生效）
	// 实际代表着生成的 buffer 区域空间为 (1/2 * blockNum) MB 即默认 bufferSize 为 8MB
	// 该数值仅能设置为 16 的倍数 非法数值将重置为默认值
	BlockNum int `config:"blockNum"`
}

type IPVPicker string

func (ipv IPVPicker) IPV4() bool {
	if ipv == "" || ipv == "v4" {
		return true
	}
	return false
}

func (ipv IPVPicker) IPV6() bool {
	if ipv == "" || ipv == "v6" {
		return true
	}
	return false
}

type ProtoRule struct {
	Name     string   `config:"name"`
	Protocol string   `config:"protocol"`
	Host     string   `config:"host"`
	Ports    []uint16 `config:"ports"`
}

func (r ProtoRule) compileBPFFilter(layer4 string) string {
	var buf strings.Builder
	buf.WriteString("(")
	buf.WriteString(layer4)

	if r.Host != "" {
		buf.WriteString(" and host ")
		buf.WriteString(r.Host)
	}

	switch len(r.Ports) {
	case 0:
		return ""

	case 1:
		buf.WriteString(" and port ")
		buf.WriteString(strconv.Itoa(int(r.Ports[0])))

	default:
		for i := 0; i < len(r.Ports); i++ {
			if i > 0 {
				buf.WriteString(" or port ")
				buf.WriteString(strconv.Itoa(int(r.Ports[i])))
				continue
			}
			buf.WriteString(" and ( port ")
			buf.WriteString(strconv.Itoa(int(r.Ports[i])))
		}
		buf.WriteString(" ) ")
	}

	buf.WriteString(")")
	return buf.String()
}

type Protocols struct {
	Rules []ProtoRule `config:"rules"`
}

// CompileBPFFilter 编译 BPF 协议语法规则
func (ps Protocols) CompileBPFFilter() (string, error) {
	var filters []string
	for _, p := range ps.Rules {
		l4, ok := socket.L7ProtoBased(socket.L7Proto(p.Protocol))
		if !ok {
			return "", errors.Errorf("unsupported protocol (%s)", p.Protocol)
		}

		filter := p.compileBPFFilter(string(l4))
		if strings.TrimSpace(filter) == "" {
			continue
		}
		filters = append(filters, filter)
	}

	if len(filters) == 0 {
		return "", nil
	}
	return strings.Join(filters, " or "), nil
}

// L7Ports 将协议规则转换为端口列表
func (ps Protocols) L7Ports() []socket.L7Ports {
	toPorts := func(ports []uint16) []socket.Port {
		dst := make([]socket.Port, 0, len(ports))
		for i := 0; i < len(ports); i++ {
			dst = append(dst, socket.Port(ports[i]))
		}
		return dst
	}

	var ports []socket.L7Ports
	for _, proto := range ps.Rules {
		ports = append(ports, socket.L7Ports{
			Ports: toPorts(proto.Ports),
			Proto: socket.L7Proto(proto.Protocol),
		})
	}
	return ports
}
