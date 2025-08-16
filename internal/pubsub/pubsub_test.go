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

package pubsub

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPubSub(t *testing.T) {
	bus := New()

	const workers = 10
	var total atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q := bus.Subscribe(10)
			defer bus.Unsubscribe(q)

			for n := 0; n < 20; n++ {
				q.Push(i)
			}

			var count int
			for {
				_, ok := q.PopTimeout(time.Second)
				if !ok {
					break
				}
				count++
			}
			total.Add(int64(count))
			assert.Equal(t, 10, count)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(100), total.Load())
	assert.Equal(t, 0, bus.Num())
}
