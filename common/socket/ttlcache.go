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

package socket

import (
	"sync"
	"time"
)

type TTLCache struct {
	mut sync.RWMutex
	set map[Tuple]time.Time

	expired time.Duration
	done    chan struct{}
}

func NewTTLCache(expired time.Duration) *TTLCache {
	tc := &TTLCache{
		set:     make(map[Tuple]time.Time),
		expired: expired,
		done:    make(chan struct{}),
	}
	go tc.gc()
	return tc
}

func (tc *TTLCache) Close() {
	close(tc.done)
}

func (tc *TTLCache) Set(tuple Tuple) {
	tc.mut.Lock()
	defer tc.mut.Unlock()

	tc.set[tuple] = time.Now().Add(tc.expired)
}

func (tc *TTLCache) Has(tuple Tuple) bool {
	tc.mut.RLock()
	defer tc.mut.RUnlock()

	v, ok := tc.set[tuple]
	if !ok {
		return false
	}
	return time.Now().Before(v)
}

func (tc *TTLCache) Count() int {
	tc.mut.RLock()
	defer tc.mut.RUnlock()

	return len(tc.set)
}

func (tc *TTLCache) gc() {
	ticker := time.NewTicker(tc.expired / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tc.mut.Lock()
			now := time.Now()
			for k, v := range tc.set {
				if now.After(v) {
					delete(tc.set, k)
				}
			}
			tc.mut.Unlock()

		case <-tc.done:
			return
		}
	}
}
