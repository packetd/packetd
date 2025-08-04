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

import (
	"github.com/spf13/cast"
)

type Options map[string]any

func NewOptions() Options {
	return make(Options)
}

func (o Options) GetInt(k string) (int, error) {
	return cast.ToIntE(o[k])
}

func (o Options) GetBool(k string) (bool, error) {
	return cast.ToBoolE(o[k])
}

func (o Options) GetStringSlice(k string) ([]string, error) {
	return cast.ToStringSliceE(o[k])
}

func (o Options) Merge(k string, v any) {
	o[k] = v
}
