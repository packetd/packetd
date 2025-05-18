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

package processor

import (
	"github.com/pkg/errors"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/confengine"
)

type Configs []Config

type Config struct {
	Name   string         `config:"name"`
	Config map[string]any `config:"config"`
}

// Processor 定义了数据处理接口的行为
//
// Processor 负责处理所有类型的 *common.Record 数据 即 roundtrips/metrics/traces
type Processor interface {
	// Name 返回处理器的名称
	Name() string

	// Process 处理 *Common.Record 数据 并返回衍生数据（如果存在的话）
	Process(*common.Record) (*common.Record, error)

	// Clean 清理资源
	Clean()
}

type CreateFunc func(conf map[string]any) (Processor, error)

var processorFactory = map[string]CreateFunc{}

func Register(name string, f CreateFunc) {
	processorFactory[name] = f
}

func Get(name string) (CreateFunc, error) {
	f, ok := processorFactory[name]
	if !ok {
		return nil, errors.Errorf("processor factory (%s) not found", name)
	}
	return f, nil
}

func loadProcessors(conf *confengine.Config) ([]Processor, error) {
	var configs Configs
	if err := conf.UnpackChild("processor", &configs); err != nil {
		return nil, err
	}

	var processors []Processor
	for _, pcfg := range configs {
		f, err := Get(pcfg.Name)
		if err != nil {
			return nil, err
		}
		con, err := f(pcfg.Config)
		if err != nil {
			return nil, err
		}
		processors = append(processors, con)
	}
	return processors, nil
}

// Manager 管理着 processor 列表 仅负责 Processor 的加载和检索
type Manager struct {
	processors []Processor
}

func NewManager(conf *confengine.Config) (*Manager, error) {
	processors, err := loadProcessors(conf)
	if err != nil {
		return nil, err
	}

	return &Manager{
		processors: processors,
	}, nil
}

func (mgr *Manager) Get(name string) (Processor, bool) {
	for _, p := range mgr.processors {
		if p.Name() == name {
			return p, true
		}
	}
	return nil, false
}
