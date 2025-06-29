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

package libpcap

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/afpacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/pkg/errors"
	"golang.org/x/net/bpf"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/logger"
	"github.com/packetd/packetd/sniffer"
)

func init() {
	sniffer.Register(New, Name, "")
}

type handler struct {
	name   string
	handle *afpacket.TPacket
	pfile  *pcap.Handle
}

type pcapSniffer struct {
	ctx        context.Context
	cancel     context.CancelFunc
	conf       *sniffer.Config
	handlers   []*handler
	wg         sync.WaitGroup
	onL4Packet sniffer.OnL4Packet
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

func (ps *pcapSniffer) L7Ports() []socket.L7Ports {
	return ps.conf.Protocols.L7Ports()
}

func (ps *pcapSniffer) SetOnL4Packet(f sniffer.OnL4Packet) {
	ps.onL4Packet = f
}

func (ps *pcapSniffer) Name() string {
	return Name
}

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
			name:  fmt.Sprintf("pcap.file: %s", ps.conf.File),
			pfile: tp,
		})
		logger.Infof("sniffer add pcap file (%s)", ps.conf.File)
		return nil
	}

	for _, iface := range ifaces {
		tp, err := ps.getTpacket(iface.Name)
		if err != nil {
			logger.Errorf("make iface (%s) *afpacket failed: %v", iface.Name, err)
			continue
		}

		if bpfFilter != "" {
			if err = ps.setBPFFilter(tp, bpfFilter); err != nil {
				tp.Close()
				return errors.Wrapf(err, "set bpf-filter (%s) failed", bpfFilter)
			}
		}

		ps.handlers = append(ps.handlers, &handler{handle: tp, name: iface.Name})
		logger.Infof("sniffer add device (%s), address=%v", iface.Name, ifaceAddress(iface))
	}

	if len(ps.handlers) == 0 {
		return errors.New("no available devices found")
	}
	return nil
}

func (ps *pcapSniffer) getTpacket(device string) (*afpacket.TPacket, error) {
	blockNumOpt := afpacket.OptNumBlocks(defaultBlockNum)
	pollTimeout := afpacket.OptPollTimeout(defaultPollTimeout)

	if device == deviceAny {
		return afpacket.NewTPacket(blockNumOpt, pollTimeout)
	}
	return afpacket.NewTPacket(afpacket.OptInterface(device), blockNumOpt, pollTimeout)
}

func (ps *pcapSniffer) setBPFFilter(tp *afpacket.TPacket, filter string) error {
	pcapBPF, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, defaultCaptureLength, filter)
	if err != nil {
		return err
	}
	var bpfIns []bpf.RawInstruction
	for _, ins := range pcapBPF {
		bpfIns = append(bpfIns, bpf.RawInstruction{
			Op: ins.Code,
			Jt: ins.Jt,
			Jf: ins.Jf,
			K:  ins.K,
		})
	}
	return tp.SetBPF(bpfIns)
}

func (ps *pcapSniffer) parsePacket(pkt []byte, ts time.Time) {
	payload, lyr, err := sniffer.DecodeIPLayer(pkt, ps.conf.IPv4Only)
	if err != nil || lyr == nil {
		return
	}

	var tcpPkt layers.TCP
	err = tcpPkt.DecodeFromBytes(payload, gopacket.NilDecodeFeedback)
	if err == nil {
		if l4pkt := sniffer.ParseTCPPacket(ts, lyr, &tcpPkt); l4pkt != nil {
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
	if l4pkt := sniffer.ParseUDPDatagram(ts, lyr, &udpPkt); l4pkt != nil {
		if ps.onL4Packet != nil {
			ps.onL4Packet(l4pkt)
		}
	}
}

func (ps *pcapSniffer) listen(ph *handler) {
	if ph.pfile != nil {
		ps.listenPcapFile(ph)
		return
	}

	ps.listenAfPacket(ph)
}

func (ps *pcapSniffer) listenAfPacket(ph *handler) {
	ps.wg.Add(1)
	defer ps.wg.Done()

	defer ph.handle.Close()

	for {
		select {
		case <-ps.ctx.Done():
			return

		default:
			pkt, ci, err := ph.handle.ZeroCopyReadPacketData()
			if err != nil {
				if errors.Is(err, pcap.NextErrorNotActivated) {
					logger.Warnf("iface (%s) not active: %v", ph.name, err)
					return
				}
				continue
			}
			ps.parsePacket(pkt, ci.Timestamp)
		}
	}
}

func (ps *pcapSniffer) listenPcapFile(ph *handler) {
	ps.wg.Add(1)
	defer ps.wg.Done()

	packetSource := gopacket.NewPacketSource(ph.pfile, ph.pfile.LinkType())
	packetSource.Lazy = true
	packetSource.NoCopy = true

	for {
		select {
		case packet, ok := <-packetSource.Packets():
			if !ok {
				logger.Infof("pcap handle (%s) closed", ph.name)
				return
			}
			ps.parsePacket(packet.Data(), time.Now())
		}
	}
}

func (ps *pcapSniffer) Reload(conf *sniffer.Config) error {
	bpfFilter, err := conf.Protocols.CompileBPFFilter()
	if err != nil {
		return err
	}
	for _, h := range ps.handlers {
		if err := ps.setBPFFilter(h.handle, bpfFilter); err != nil {
			return err
		}
	}
	ps.conf = conf
	return nil
}

func (ps *pcapSniffer) Close() {
	ps.cancel()
	ps.wg.Wait()
}

// filterInterfaces 过滤指定网卡
//
// 同一块网卡可能同时包含多个 IP 地址 v4/v6 所以这里只做初步筛选 允许筛除只含 ipv6 地址的网卡
func filterInterfaces(pattern string, hasIPv4 bool) ([]net.Interface, error) {
	if pattern == "any" {
		return []net.Interface{{Name: "any"}}, nil
	}
	if pattern == "" {
		pattern = ".*"
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
		if r.MatchString(iface.Name) {
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
