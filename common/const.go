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

package common

const (
	// App 应用程序名称
	App = "packetd"

	// Version 应用程序版本
	Version = "v0.0.1"

	// ReadWriteBlockSize 默认的 ringbuffer 的长度
	//
	// TCP Segments 的最大长度为 64K (65535 bytes)
	// 但如果对于每条链接的双向 Stream 都创建这么一大块空间会造成过多的开销
	// 所以可以设置一个`折中的` buffersize 但这就会要求对 Segment Payload 进行切割
	ReadWriteBlockSize = 4096
)
