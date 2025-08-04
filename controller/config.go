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

import "time"

type Config struct {
	// Layer4Metrics 四层指标统计
	Layer4Metrics struct {
		Enabled        bool     `config:"enabled"`
		RequiredLabels []string `config:"requiredLabels"`
	} `config:"layer4Metrics"`

	// ConnExpired 未活跃链接过期时间
	ConnExpired time.Duration `config:"connExpired"`

	// Decoder 指定每种 decoder 解析特性
	Decoder DecoderConfig `config:"decoder"`
}

func (c Config) GetConnExpired() time.Duration {
	if c.ConnExpired < time.Minute {
		return 5 * time.Minute
	}
	return c.ConnExpired
}

type DecoderConfig struct {
	MongoDB map[string]any `config:"mongodb"`
}

func (c DecoderConfig) Get(proto string) map[string]any {
	switch proto {
	case "mongodb":
		return c.MongoDB
	}
	return nil
}
