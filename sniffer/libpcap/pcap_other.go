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

//go:build !linux

package libpcap

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/pkg/errors"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/logger"
	"github.com/packetd/packetd/sniffer"
)

func init() {
	sniffer.Register(New, Name, "")
}

type handler struct {
	name   string
	handle *pcap.Handle
}

type pcapSniffer struct {
	ctx        context.Context
	cancel     context.CancelFunc
	conf       *sniffer.Config
	handlers   []*handler
	wg         sync.WaitGroup
	onL4Packet sniffer.OnL4Packet
}

func (ps *pcapSniffer) Name() string {
	return Name
}

func (ps *pcapSniffer) SetOnL4Packet(f sniffer.OnL4Packet) {
	ps.onL4Packet = f
}

func (ps *pcapSniffer) L7Ports() []socket.L7Ports {
	return ps.conf.Protocols.L7Ports()
}

func New(conf *sniffer.Config) (sniffer.Sniffer, error) {
	snif := &pcapSniffer{
		conf: conf,
	}

	snif.ctx, snif.cancel = context.WithCancel(context.Background())
	if err := snif.makeHandlers(); err != nil {
		return nil, err
	}

	for _, h := range snif.handlers {
		go snif.listen(h)
	}

	return snif, nil
}

// makeHandlers 创建设备监听句柄
//
// TODO(mando): 非 linux 系统不支持 'any' 网卡 即启动时就已经决定了使用的设备
// 后续不再更新 因此这里需要有一个 watch/poll 机制来保证新增的设备能被处理
func (ps *pcapSniffer) makeHandlers() error {
	ifaces, err := filterInterfaces(ps.conf.Ifaces, ps.conf.IPv4Only)
	if err != nil {
		return err
	}

	bpfFilter, err := ps.conf.Protocols.CompileBPFFilter()
	if err != nil {
		return err
	}

	if len(ps.conf.File) > 0 {
		tp, err := makeFileHandle(ps.conf.File, bpfFilter)
		if err != nil {
			return err
		}
		ps.handlers = append(ps.handlers, &handler{
			name:   fmt.Sprintf("pcap.file: %s", ps.conf.File),
			handle: tp,
		})
		logger.Infof("sniffer add pcap file (%s)", ps.conf.File)
		return nil
	}

	for _, iface := range ifaces {
		tp, err := ps.getHandle(iface.Name, bpfFilter)
		if err != nil {
			logger.Errorf("make iface (%s) *TPPacket failed: %v", iface.Name, err)
			continue
		}

		ps.handlers = append(ps.handlers, &handler{
			name:   fmt.Sprintf("pcap.device: %s", iface.Name),
			handle: tp,
		})
		logger.Infof("sniffer add device (%s), address=%v", iface.Name, ifaceAddress(iface))
	}

	if len(ps.handlers) == 0 {
		return errors.New("no available devices found")
	}
	return nil
}

func (ps *pcapSniffer) getHandle(device, bpfFilter string) (*pcap.Handle, error) {
	handle, err := pcap.OpenLive(device, socket.MaxIPPacketSize, !ps.conf.NoPromiscuous, pcap.BlockForever)
	if err != nil {
		return nil, err
	}

	if bpfFilter != "" {
		if err := handle.SetBPFFilter(bpfFilter); err != nil {
			handle.Close()
			return nil, errors.Wrapf(err, "set bpf-filter (%s) failed", bpfFilter)
		}
	}
	return handle, nil
}

func (ps *pcapSniffer) parsePacket(packet gopacket.Packet) {
	payload, lyr, err := sniffer.DecodeIPLayer(packet.Data(), ps.conf.IPv4Only)
	if err != nil {
		return
	}

	var tcpPkt layers.TCP
	err = tcpPkt.DecodeFromBytes(payload, gopacket.NilDecodeFeedback)
	if err == nil {
		if l4pkt := sniffer.ParseTCPPacket(time.Now(), lyr, &tcpPkt); l4pkt != nil {
			if ps.onL4Packet != nil {
				ps.onL4Packet(l4pkt)
			}
		}
		return
	}

	var udpPkt layers.UDP
	err = udpPkt.DecodeFromBytes(payload, gopacket.NilDecodeFeedback)
	if err != nil {
		return
	}
	if l4pkt := sniffer.ParseUDPDatagram(time.Now(), lyr, &udpPkt); l4pkt != nil {
		if ps.onL4Packet != nil {
			ps.onL4Packet(l4pkt)
		}
	}
}

func (ps *pcapSniffer) listen(ph *handler) {
	ps.wg.Add(1)
	defer ps.wg.Done()

	packetSource := gopacket.NewPacketSource(ph.handle, ph.handle.LinkType())
	packetSource.Lazy = true
	packetSource.NoCopy = true

	for {
		select {
		case packet, ok := <-packetSource.Packets():
			if !ok {
				logger.Infof("pcap handle (%s) closed", ph.name)
				return
			}
			ps.parsePacket(packet)
		}
	}
}

func (ps *pcapSniffer) Reload(conf *sniffer.Config) error {
	bpfFilter, err := conf.Protocols.CompileBPFFilter()
	if err != nil {
		return err
	}
	for _, h := range ps.handlers {
		if err := h.handle.SetBPFFilter(bpfFilter); err != nil {
			return err
		}
	}
	return nil
}

func (ps *pcapSniffer) Close() {
	for _, h := range ps.handlers {
		h.handle.Close()
	}
	ps.wg.Wait()
}

// filterInterfaces 过滤指定网卡
//
// 同一块网卡可能同时包含多个 IP 地址 v4/v6 所以这里只做初步筛选 允许筛除只含 ipv6 地址的网卡
func filterInterfaces(pattern string, hasIPv4 bool) ([]net.Interface, error) {
	var all bool
	if pattern == "" || pattern == "any" {
		all = true // 代表监听所有网卡
	}

	var matched []net.Interface
	r, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range ifaces {
		if r.MatchString(iface.Name) || all {
			if hasIPv4 && !hasIPv4Addr(iface) {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil || len(addrs) == 0 {
				continue
			}
			matched = append(matched, iface)
		}
	}
	return matched, nil
}
