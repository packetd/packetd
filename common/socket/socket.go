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

package socket

import (
	"fmt"
	"net"
	"time"
)

const (
	// MaxIPPacketSize IP 数据包最大大小
	//
	// 实际上没有办法达到这个数量 IP 最大理论长度为 2^16-1 Bytes 而 TCP
	// 但此数值可以保证一定可以读到完整的 TCP 包
	MaxIPPacketSize = 65535

	// TCPMsl 最长报文周期（Maximum Segment Lifetime）
	//
	// https://datatracker.ietf.org/doc/html/rfc9293#section-3.4.2-2
	// For this specification the MSL is taken to be 2 minutes.
	// This is an engineering choice, and may be changed if experience indicates it is desirable to do so.
	//
	// 任何的 TCP 实现都需要给 MSL 设置一个默认值 最新的 TCP 协议标准对此的建议值是 2min
	// 不同操作系统的默认 MSL
	// * Linux: 1min
	// * BSD: 30s
	// * Windows: 2min
	//
	// 因此 TIME-WAIT 状态的持续时间通常在 1 分钟到 4 分钟之间
	TCPMsl = time.Minute
)

// Version IP 版本 v4/v6
type Version uint8

const (
	V4 Version = iota
	V6
)

// IPV 基于 net.IP 做了一层封装
//
// 记录了 IP Bytes 以及协议版本信息
type IPV struct {
	IP      [net.IPv6len]byte
	Version Version
}

// ToIPV4 将 net.IP 转换为 IPV4 版本
func ToIPV4(ip net.IP) IPV {
	var dst [net.IPv6len]byte
	copy(dst[:], ip[:])
	return IPV{
		IP:      dst,
		Version: V4,
	}
}

// ToIPV6 将 net.IP 转换为 IPV6 版本
func ToIPV6(ip net.IP) IPV {
	var dst [net.IPv6len]byte
	copy(dst[:], ip[:])
	return IPV{
		IP:      dst,
		Version: V6,
	}
}

// NetIP 将 IPV 转换为 net.IP
func (ipv IPV) NetIP() net.IP {
	if ipv.Version == V4 {
		return ipv.IP[:net.IPv4len]
	}
	return ipv.IP[:]
}

func (ipv IPV) String() string {
	return ipv.NetIP().String()
}

type Port uint16

// Tuple 四元组标识
//
// 对于全双工链接来说 并无准确的源 IP 目标 IP 的说法 但 Socket 本身是有方向的
type Tuple struct {
	SrcIP   IPV
	DstIP   IPV
	SrcPort Port
	DstPort Port
}

func (t Tuple) ToRaw() TupleRaw {
	return TupleRaw{
		SrcIP:   t.SrcIP.String(),
		DstIP:   t.DstIP.String(),
		SrcPort: uint16(t.SrcPort),
		DstPort: uint16(t.DstPort),
	}
}

func (t Tuple) String() string {
	return fmt.Sprintf("%s:%d > %s:%d", t.SrcIP, t.SrcPort, t.DstIP, t.DstPort)
}

// TupleRaw 将四元组转换成原始数据格式
type TupleRaw struct {
	SrcIP   string
	DstIP   string
	SrcPort uint16
	DstPort uint16
}

func (t TupleRaw) String() string {
	return fmt.Sprintf("%s:%d > %s:%d", t.SrcIP, t.SrcPort, t.DstIP, t.DstPort)
}

// Mirror 反转链接 即通信的另一端
func (t Tuple) Mirror() Tuple {
	return Tuple{
		SrcIP:   t.DstIP,
		DstIP:   t.SrcIP,
		SrcPort: t.DstPort,
		DstPort: t.SrcPort,
	}
}

// L4Proto Layer4 传输层协议 即 TCP/UDP
type L4Proto string

const (
	L4ProtoTCP L4Proto = "tcp"
	L4ProtoUDP L4Proto = "udp"
)

// L7Proto Layer7 应用层协议
type L7Proto string

const (
	L7ProtoHTTP       L7Proto = "http"
	L7ProtoRedis      L7Proto = "redis"
	L7ProtoMySQL      L7Proto = "mysql"
	L7ProtoHTTP2      L7Proto = "http2"
	L7ProtoGRPC       L7Proto = "grpc"
	L7ProtoDNS        L7Proto = "dns"
	L7ProtoMongoDB    L7Proto = "mongodb"
	L7ProtoPostgreSQL L7Proto = "postgresql"
	L7ProtoKafka      L7Proto = "kafka"
	L7ProtoAMQP       L7Proto = "amqp"
)

func L7ProtoBased(l7 L7Proto) (L4Proto, bool) {
	protos := map[L7Proto]L4Proto{
		L7ProtoHTTP:       L4ProtoTCP,
		L7ProtoRedis:      L4ProtoTCP,
		L7ProtoMySQL:      L4ProtoTCP,
		L7ProtoHTTP2:      L4ProtoTCP,
		L7ProtoGRPC:       L4ProtoTCP,
		L7ProtoDNS:        L4ProtoUDP,
		L7ProtoMongoDB:    L4ProtoTCP,
		L7ProtoPostgreSQL: L4ProtoTCP,
		L7ProtoKafka:      L4ProtoTCP,
		L7ProtoAMQP:       L4ProtoTCP,
	}

	v, ok := protos[l7]
	return v, ok
}

// L7Ports 应用层端口列表
type L7Ports struct {
	Ports []Port
	Proto L7Proto
}
