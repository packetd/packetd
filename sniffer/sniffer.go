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
	"runtime"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/pkg/errors"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/confengine"
)

// OnL4Packet 触发 L4Packet 的解析回调
type OnL4Packet func(pkt socket.L4Packet)

// Stats 统计数据
type Stats struct {
	Name    string // 设备名称
	Packets uint   // 收包数量
	Drops   uint   // 丢包数量
}

// Sniffer 负责实现网络数据包的嗅探并调用 On* 函数进行处理
type Sniffer interface {
	// Name 返回 Sniffer 名称
	Name() string

	// Reload 动态重载配置参数
	Reload(conf *Config) error

	// SetOnL4Packet 设置 OnL4Packet 回调函数
	SetOnL4Packet(f OnL4Packet)

	// L7Ports 返回当前应用层端口及协议列表
	L7Ports() []socket.L7Ports

	// Stats 返回 sniffer 统计数据
	Stats() []Stats

	// Close 关闭 Sniffer 并释放关联资源
	Close()
}

// CreateFunc 创建 Sniffer 的函数类型
type CreateFunc func(conf *Config) (Sniffer, error)

var snifferFactory = map[string]CreateFunc{}

// Register 注册 Sniffer 工厂函数
func Register(f CreateFunc, names ...string) {
	for _, name := range names {
		snifferFactory[name] = f
	}
}

// Get 获取 Sniffer 工厂函数
func Get(name string) (CreateFunc, error) {
	f, ok := snifferFactory[name]
	if !ok {
		return nil, errors.Errorf("sniffer factory (%s) not found", name)
	}
	return f, nil
}

func New(conf *confengine.Config) (Sniffer, error) {
	var cfg Config
	if err := conf.UnpackChild("sniffer", &cfg); err != nil {
		return nil, err
	}

	if cfg.Engine == "" {
		cfg.Engine = "pcap"
	}

	f, err := Get(cfg.Engine)
	if err != nil {
		return nil, err
	}
	return f(&cfg)
}

// ParseTCPPacket 解析 TCP 数据包
//
// 支持定义解析 Layer 类型
func ParseTCPPacket(ts time.Time, lyrs ...gopacket.Layer) *socket.TCPSegment {
	pkt := parsePacket(ts, lyrs...)
	if pkt == nil {
		return nil
	}

	tcp, ok := pkt.(*socket.TCPSegment)
	if !ok {
		return nil
	}
	return tcp
}

// ParseUDPDatagram 解析 UDP 数据包
//
// 支持定义解析 Layer 类型
func ParseUDPDatagram(ts time.Time, lyrs ...gopacket.Layer) *socket.UDPDatagram {
	pkt := parsePacket(ts, lyrs...)
	if pkt == nil {
		return nil
	}

	udp, ok := pkt.(*socket.UDPDatagram)
	if !ok {
		return nil
	}
	return udp
}

func parsePacket(ts time.Time, lyrs ...gopacket.Layer) socket.L4Packet {
	var srcPort, dstPort socket.Port
	var srcIP, dstIP socket.IPV
	var protocol socket.L4Proto
	var payload []byte

	// TCP 字段
	var seq uint32
	var finFlag bool

	for _, layerType := range lyrs {
		switch lyr := layerType.(type) {
		case *layers.IPv4:
			srcIP = socket.ToIPV4(lyr.SrcIP)
			dstIP = socket.ToIPV4(lyr.DstIP)

		case *layers.IPv6:
			srcIP = socket.ToIPV6(lyr.SrcIP)
			dstIP = socket.ToIPV6(lyr.DstIP)

		case *layers.TCP:
			protocol = socket.L4ProtoTCP
			srcPort = socket.Port(lyr.SrcPort)
			dstPort = socket.Port(lyr.DstPort)
			payload = lyr.Payload
			seq = lyr.Seq
			finFlag = lyr.FIN

		case *layers.UDP:
			protocol = socket.L4ProtoUDP
			srcPort = socket.Port(lyr.SrcPort)
			dstPort = socket.Port(lyr.DstPort)
			payload = lyr.Payload
		}
	}

	switch protocol {
	case socket.L4ProtoTCP:
		return &socket.TCPSegment{
			Time:    ts,
			Seq:     seq,
			FIN:     finFlag,
			Payload: payload,
			Tuple: socket.Tuple{
				SrcIP:   srcIP,
				SrcPort: srcPort,
				DstIP:   dstIP,
				DstPort: dstPort,
			},
		}
	case socket.L4ProtoUDP:
		return &socket.UDPDatagram{
			Time:    ts,
			Payload: payload,
			Tuple: socket.Tuple{
				SrcIP:   srcIP,
				SrcPort: srcPort,
				DstIP:   dstIP,
				DstPort: dstPort,
			},
		}
	default:
		return nil
	}
}

// DecodeIPLayer 解析 IP 层
//
// 返回数据包 Payload 以及所处 Layer
func DecodeIPLayer(b []byte, ipvPicker IPVPicker) ([]byte, gopacket.Layer, gopacket.LayerType, error) {
	// decode packets followed by layers
	// 1) Ethernet Layer
	// 2) IP Layer
	// 3) TCP/UDP Layer
	content, ipv, err := decodeIPLayer(b)
	if err != nil {
		return nil, nil, 0, err
	}

	var lyr gopacket.Layer
	var next gopacket.LayerType
	var payload []byte

	switch ipv {
	case layerIpv4:
		if !ipvPicker.IPV4() {
			return nil, nil, 0, nil
		}

		var ipLayer layers.IPv4
		err := ipLayer.DecodeFromBytes(content, gopacket.NilDecodeFeedback)
		if err != nil {
			return nil, nil, 0, err
		}

		payload = ipLayer.Payload
		lyr = &ipLayer
		next = ipLayer.NextLayerType()

	case layerIpv6:
		if !ipvPicker.IPV6() {
			return nil, nil, 0, nil
		}

		var ipLayer layers.IPv6
		err := ipLayer.DecodeFromBytes(content, gopacket.NilDecodeFeedback)
		if err != nil {
			return nil, nil, 0, err
		}

		payload = ipLayer.Payload
		lyr = &ipLayer
		next = ipLayer.NextLayerType()
	}

	if lyr == nil || len(payload) == 0 {
		return nil, nil, 0, nil
	}

	return payload, lyr, next, nil
}

const (
	layerIpv4 uint8 = iota
	layerIpv6
)

// decodeIPLayer 解析 IP 层协议
//
// OpenBSD style 系统需要额外判断处理 loopback 网卡
func decodeIPLayer(b []byte) ([]byte, uint8, error) {
	var err error
	var ether layers.Ethernet
	if err = ether.DecodeFromBytes(b, gopacket.NilDecodeFeedback); err == nil {
		switch ether.EthernetType {
		case layers.EthernetTypeIPv4:
			return ether.Payload, layerIpv4, nil
		case layers.EthernetTypeIPv6:
			return ether.Payload, layerIpv6, nil
		}
	}

	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		var lb layers.Loopback
		err = lb.DecodeFromBytes(b, gopacket.NilDecodeFeedback)
		if err != nil {
			return nil, 0, err
		}
		switch lb.NextLayerType() {
		case layers.LayerTypeIPv4:
			return lb.Payload, layerIpv4, nil
		case layers.LayerTypeIPv6:
			return lb.Payload, layerIpv6, nil
		default:
			return nil, 0, errors.New("unknown loopback nextLayer")
		}
	}
	return nil, 0, err
}
