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
	"net/url"
	"time"
)

const defaultTimeout = 15 * time.Second

type Config struct {
	Traces     TracesConfig     `config:"traces"`
	Metrics    MetricsConfig    `config:"metrics"`
	RoundTrips RoundTripsConfig `config:"roundtrips"`
}

type TracesConfig struct {
	Enabled  bool              `config:"enabled"`
	Batch    int               `config:"batch"`
	Endpoint string            `config:"endpoint"`
	Header   map[string]string `config:"header"`
	Interval time.Duration     `config:"interval"`
	Timeout  time.Duration     `config:"timeout"`
}

func (tc *TracesConfig) Validate() error {
	_, err := url.Parse(tc.Endpoint)
	if err != nil {
		return err
	}

	if tc.Batch <= 0 {
		tc.Batch = 100
	}
	if tc.Timeout <= 0 {
		tc.Timeout = defaultTimeout
	}
	if tc.Interval <= 0 {
		tc.Interval = 3 * time.Second
	}
	return nil
}

type MetricsConfig struct {
	Enabled  bool              `config:"enabled"`
	Endpoint string            `config:"endpoint"`
	Header   map[string]string `config:"header"`
	Interval time.Duration     `config:"interval"`
	Timeout  time.Duration     `config:"timeout"`
}

func (mc *MetricsConfig) Validate() error {
	_, err := url.Parse(mc.Endpoint)
	if err != nil {
		return err
	}

	if mc.Timeout <= 0 {
		mc.Timeout = defaultTimeout
	}
	if mc.Interval <= 0 {
		mc.Interval = time.Minute
	}
	return nil
}

type RoundTripsConfig struct {
	Enabled    bool   `config:"enabled"`
	Console    bool   `config:"console"`
	Filename   string `config:"filename"`
	MaxSize    int    `config:"maxSize"`
	MaxBackups int    `config:"maxBackups"`
	MaxAge     int    `config:"maxAge"`
}

func (rc *RoundTripsConfig) Validate() {
	if rc.Filename == "" {
		rc.Filename = "roundtrips.log"
	}
	if rc.MaxSize <= 0 {
		rc.MaxSize = 100
	}
	if rc.MaxAge <= 0 {
		rc.MaxAge = 7
	}
	if rc.MaxBackups <= 0 {
		rc.MaxBackups = 10
	}
}
