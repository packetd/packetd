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

package controller

import (
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/connstream"
	"github.com/packetd/packetd/protocol"
)

// portPools 记录了端口与协议池的映射关系
//
// 可通过 Reload 重新加载配置 pps 会对比新旧配置进行更新
type portPools struct {
	ports map[socket.Port]socket.L7Proto
	pools map[socket.L7Proto]protocol.ConnPool
}

func newPortPools(l7ports []socket.L7Ports) (*portPools, error) {
	ports := make(map[socket.Port]socket.L7Proto)
	pools := make(map[socket.L7Proto]protocol.ConnPool)

	for _, pp := range l7ports {
		for _, port := range pp.Ports {
			ports[port] = pp.Proto
			if _, ok := pools[pp.Proto]; !ok {
				f, err := protocol.Get(pp.Proto)
				if err != nil {
					return nil, err
				}
				pools[pp.Proto] = f()
			}
		}
	}

	return &portPools{
		ports: ports,
		pools: pools,
	}, nil
}

func (pps *portPools) Reload(l7ports []socket.L7Ports) error {
	newPorts := make(map[socket.Port]socket.L7Proto)
	newProto := make(map[socket.L7Proto]struct{})

	for _, pp := range l7ports {
		for _, port := range pp.Ports {
			newPorts[port] = pp.Proto
			newProto[pp.Proto] = struct{}{}
		}
	}

	// 之前不存在的 protocol 标记为新增
	// 存在则继续保持
	added := make(map[socket.L7Proto]struct{})
	newPools := make(map[socket.L7Proto]protocol.ConnPool)
	for p := range newProto {
		if conn, ok := pps.pools[p]; !ok {
			added[p] = struct{}{}
		} else {
			newPools[p] = conn
		}
	}

	// 新配置中不存在的 protocol 标记为删除
	deleted := make(map[socket.L7Proto]struct{})
	for p := range pps.pools {
		if _, ok := newProto[p]; !ok {
			deleted[p] = struct{}{}
		}
	}

	var errs error
	for p := range added {
		f, err := protocol.Get(p)
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}
		newPools[p] = f()
	}

	prev := pps.pools
	pps.pools = newPools
	pps.ports = newPorts

	for p := range deleted {
		prev[p].Clean()
	}
	return errs
}

func (pps *portPools) DecideProto(st socket.Tuple) (socket.Port, protocol.ConnPool) {
	if p, ok := pps.ports[st.SrcPort]; ok {
		return st.SrcPort, pps.pools[p]
	}
	if p, ok := pps.ports[st.DstPort]; ok {
		return st.DstPort, pps.pools[p]
	}
	return 0, nil
}

func (pps *portPools) RangePoolStats(f func(stats connstream.TupleStats)) {
	for _, pool := range pps.pools {
		pool.OnStats(func(stats connstream.TupleStats) {
			f(stats)
		})
	}
}

func (pps *portPools) RemoveExpired(duration time.Duration) {
	for _, pool := range pps.pools {
		pool.RemoveExpired(duration)
	}
}
