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

package exporter

import (
	"github.com/packetd/packetd/common"
)

// Sinker 负责将数据 `写入` 到指定存储中
type Sinker interface {
	// Name Sinker 名称 实际为 record 类型
	Name() common.RecordType

	// Sink 写入函数
	Sink(data any) error

	// Close 关闭并进行资源清理
	Close()
}

type CreateFunc func(Config) (Sinker, error)

var sinkFactory = map[common.RecordType]CreateFunc{}

func Get(name common.RecordType) CreateFunc {
	return sinkFactory[name]
}

func Register(name common.RecordType, createFunc CreateFunc) {
	sinkFactory[name] = createFunc
}
