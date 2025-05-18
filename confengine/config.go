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

package confengine

import (
	"fmt"

	"github.com/elastic/go-ucfg"
	"github.com/elastic/go-ucfg/yaml"
)

// Config 是对 ucfg.Config 的封装 并提供一些简便的操作函数
type Config struct {
	conf *ucfg.Config
}

func New(conf *ucfg.Config) *Config {
	return &Config{conf: conf}
}

func (c *Config) Has(s string) bool {
	ok, err := c.conf.Has(s, -1)
	if err != nil {
		return false
	}
	return ok
}

func (c *Config) Child(s string) (*Config, error) {
	content, err := c.conf.Child(s, -1)
	if err != nil {
		return nil, err
	}
	return &Config{conf: content}, nil
}

func (c *Config) MustChild(s string) *Config {
	child, err := c.Child(s)
	if err != nil {
		panic(err)
	}
	return child
}

func (c *Config) Unpack(to any) error {
	return c.conf.Unpack(to)
}

func (c *Config) Disabled(s string) bool {
	ok, err := c.conf.Bool(fmt.Sprintf("%s.disabled", s), -1)
	if err != nil {
		return false
	}
	return ok
}

func (c *Config) Enabled(s string) bool {
	ok, err := c.conf.Bool(fmt.Sprintf("%s.enabled", s), -1)
	if err != nil {
		return false
	}
	return ok
}

func (c *Config) UnpackChild(s string, to any) error {
	content, err := c.conf.Child(s, -1)
	if err != nil {
		return err
	}
	return content.Unpack(to)
}

func LoadConfigPath(path string) (*Config, error) {
	config, err := yaml.NewConfigWithFile(path, ucfg.PathSep("."))
	if err != nil {
		return nil, err
	}

	return New(config), err
}

func LoadContent(b []byte) (*Config, error) {
	config, err := yaml.NewConfig(b)
	if err != nil {
		return nil, err
	}
	return New(config), err
}
