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
	"runtime"
	"time"
)

var coreNums = runtime.NumCPU()

func Concurrency() int {
	return coreNums * 2
}

var started int64

func init() {
	started = time.Now().Unix()
}

// Started 返回进程启动时间戳
func Started() int64 {
	return started
}
