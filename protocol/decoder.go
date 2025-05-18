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

	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol/role"
)

// Decoder 协议解码器定义
//
// 所有的 7 层协议解码器都需实现本接口 要求实现方支持流式解析数据
type Decoder interface {
	// Decode 解析数据 不允许修改任何 Reader 读取到的任何字节
	// 如果有修改需求 请先 copy 一份
	//
	// r 是 L4 `已经切割的数据流`
	// t 为数据包被抓取的时间
	//
	// 同有一个数据包可能会解析出多个 *role.Object
	Decode(r zerocopy.Reader, t time.Time) ([]*role.Object, error)

	// Free 释放持有的资源
	Free()
}
