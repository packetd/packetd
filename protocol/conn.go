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

package protocol

import (
	"time"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/connstream"
)

// Conn 代表着一个具体协议的应用层的链接
type Conn interface {
	// OnL4Packet 如果正确处理 Layer4 的数据包
	// 返回是否写入成功
	OnL4Packet(seg socket.L4Packet, ch chan<- socket.RoundTrip) error

	// Stats 返回 Conn 统计数据
	Stats() []connstream.TupleStats

	// Free 释放链接相关资源
	Free()

	// IsClosed 返回链接是否关闭
	IsClosed() bool

	// ActiveAt 返回链接最后活跃时间
	ActiveAt() time.Time
}
