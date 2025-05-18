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

package fasttime

import (
	"sync/atomic"
	"time"
)

func init() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for tm := range ticker.C {
			t := tm.Unix()
			atomic.StoreInt64(&currentTimestamp, t)
		}
	}()
}

var currentTimestamp = time.Now().Unix()

// UnixTimestamp 获取当前 unix 时间戳 性能更快
func UnixTimestamp() int64 {
	return atomic.LoadInt64(&currentTimestamp)
}
