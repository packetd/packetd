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
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Queue PubSub 返回的订阅队列实例
type Queue interface {
	// ID 队列唯一标识
	ID() string

	// PopTimeout 从队列中弹出一个元素 操作会 block 直到有元素或者超时
	PopTimeout(timeout time.Duration) (any, bool)

	// Push 推送一个元素至队列中
	Push(data any)

	// Close 关闭并清理队列
	Close()
}

// channel 为 Queue 的一种实现
type channel struct {
	id     string
	ch     chan any
	closed atomic.Bool
}

func newChannel(size int) Queue {
	if size <= 0 {
		size = 1
	}

	return &channel{
		id: uuid.New().String(),
		ch: make(chan any, size),
	}
}

func (ch *channel) ID() string {
	return ch.id
}

func (ch *channel) PopTimeout(timeout time.Duration) (any, bool) {
	if ch.closed.Load() {
		return nil, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case data, ok := <-ch.ch:
		return data, ok

	case <-ctx.Done():
		return nil, false
	}
}

func (ch *channel) Push(data any) {
	if ch.closed.Load() {
		return
	}

	select {
	case ch.ch <- data:
	default:
	}
}

func (ch *channel) Close() {
	if ch.closed.CompareAndSwap(false, true) {
		close(ch.ch)
	}
}

type PubSub struct {
	mut    sync.RWMutex
	queues map[string]Queue
}

func New() *PubSub {
	return &PubSub{
		queues: make(map[string]Queue),
	}
}

func (p *PubSub) Num() int {
	p.mut.RLock()
	defer p.mut.RUnlock()

	return len(p.queues)
}

func (p *PubSub) Subscribe(size int) Queue {
	p.mut.Lock()
	defer p.mut.Unlock()

	ch := newChannel(size)
	p.queues[ch.ID()] = ch
	return ch
}

func (p *PubSub) Publish(msg any) {
	p.mut.RLock()
	defer p.mut.RUnlock()

	for _, q := range p.queues {
		q.Push(msg)
	}
}

func (p *PubSub) Unsubscribe(q Queue) {
	p.mut.Lock()
	defer p.mut.Unlock()

	delete(p.queues, q.ID())
}
