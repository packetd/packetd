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
	"net"
	"time"

	"github.com/gopacket/gopacket/pcap"
	"github.com/pkg/errors"

	"github.com/packetd/packetd/common/socket"
)

const (
	Name = "pcap"
)

const (
	// defaultBlockNum 默认的 block 数量
	//
	// 实际代表着生成的 buffer 区域空间为 (1/2 * blockNum) MB
	defaultBlockNum = 16

	// defaultPollTimeout 默认的 block 超时时间
	defaultPollTimeout = 500 * time.Millisecond

	// deviceAny 表示监听所有网卡
	//
	// 只在 Linux 平台生效
	deviceAny = "any"

	// defaultCaptureLength 默认的捕获长度
	//
	// 上限即 IP 包的最大长度
	defaultCaptureLength = socket.MaxIPPacketSize
)

func hasIPv4Addr(iface net.Interface) bool {
	addrs, err := iface.Addrs()
	if err != nil || len(addrs) == 0 {
		return false
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ip != nil && ip.To4() != nil {
			return true
		}
	}
	return false
}

func ifaceAddress(iface net.Interface) []string {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil
	}

	var s []string
	for _, addr := range addrs {
		s = append(s, addr.String())
	}
	return s
}

func makeFileHandle(path, bpfFilter string) (*pcap.Handle, error) {
	handle, err := pcap.OpenOffline(path)
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
