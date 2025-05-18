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

package pipeline

import (
	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/processor"
)

type Config struct {
	Name       string   `config:"name"`
	Processors []string `config:"processors"`
}

type Configs []Config

type Pipeline struct {
	configs Configs
	psmgr   *processor.Manager
}

func New(conf *confengine.Config) (*Pipeline, error) {
	configs, err := loadPipeline(conf)
	if err != nil {
		return nil, err
	}

	psmgr, err := processor.NewManager(conf)
	if err != nil {
		return nil, err
	}
	return &Pipeline{
		configs: configs,
		psmgr:   psmgr,
	}, nil
}

func (p *Pipeline) Range(src *common.Record, f func(dst *common.Record)) {
	for i := 0; i < len(p.configs); i++ {
		for _, name := range p.configs[i].Processors {
			ps, ok := p.psmgr.Get(name)
			if !ok {
				continue
			}
			r, err := ps.Process(src)
			if err != nil {
				continue
			}
			if r != nil {
				f(r)
			}
		}
	}
}

func loadPipeline(conf *confengine.Config) (Configs, error) {
	var configs Configs
	if err := conf.UnpackChild("pipeline", &configs); err != nil {
		return nil, err
	}
	return configs, nil
}
