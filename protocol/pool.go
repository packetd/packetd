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

package protocol

import (
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/connstream"
	"github.com/packetd/packetd/internal/zerocopy"
	"github.com/packetd/packetd/protocol/role"
)

var ErrConnClosed = errors.New("connection closed")

type CreateConnPool func() ConnPool

var poolFactory = map[socket.L7Proto]CreateConnPool{}

// Register 注册 ConnPool 实现函数
func Register(name socket.L7Proto, f CreateConnPool) {
	poolFactory[name] = f
}

// Get 获取 ConnPool 实现函数
func Get(name socket.L7Proto) (CreateConnPool, error) {
	f, ok := poolFactory[name]
	if !ok {
		return nil, errors.Errorf("connpool factory (%s) not found", name)
	}
	return f, nil
}

// Conn 代表着一个具体协议的应用层的链接
type Conn interface {
	// OnL4Packet 如果正确处理 Layer4 的数据包
	// 返回是否写入成功
	OnL4Packet(seg socket.L4Packet, ch chan<- socket.RoundTrip) error

	// Free 释放链接相关资源
	Free()

	// Stats 返回 Conn 统计数据
	Stats() []connstream.TupleStats

	// IsClosed 返回链接是否关闭
	IsClosed() bool

	// ActiveAt 返回链接最后活跃时间
	ActiveAt() time.Time
}

// CreateConnFunc 根据传入的 socket.Tuple 创建对应协议的 Conn
type CreateConnFunc func(st socket.Tuple, serverPort socket.Port) Conn

// ConnPool 定义了 ConnPool 应该实现的接口
//
// Pool 应该具备删除和创建链接的能力 同时执行 Clean 时应该释放链接持有的任何资源
type ConnPool interface {
	// L4Proto 返回链接 Layer Protocol 类型
	L4Proto() socket.L4Proto

	// Delete 删除一个链接
	Delete(st socket.Tuple)

	// GetOrCreate 获取或创建一个新链接
	GetOrCreate(st socket.Tuple, serverPort socket.Port) Conn

	// OnStats 触发 Stats 统计事件
	//
	// 读取即重置 counter 需要重新计数
	OnStats(f func(connstream.TupleStats))

	// ActiveConns 返回活跃的 Connection 数量
	ActiveConns() int

	// RemoveExpired 清理超过 duration 未收到任何数据包的 Conn
	RemoveExpired(duration time.Duration)

	// Clean 释放链接资源
	Clean()
}

// connPool 实现了通用的链接管理池 负责链接的创建和释放
//
// frozen 存储了已经标记为 Closed 状态的链接 用于正确处理链接销毁边界情况
//
// TCP 协议中 链接的结束需要有四次挥手的过程 如下图所示
//
//	           Client                           Server
//	           |                                |
//	           |          FIN (seq=x)           |
//	FIN_WAIT_1 | -----------------------------> |
//	           |                                | CLOSE_WAIT
//	           |         ACK (ack=x+1)          |
//	FIN_WAIT_2 | <----------------------------- |
//	           |                                |
//	           |          FIN (seq=y)           |
//	           | <----------------------------- | LAST_ACK
//	           |                                |
//	           |         ACK (ack=y+1)          |
//	 TIME_WAIT | -----------------------------> |
//	           |                                | CLOSED
//
// conn 在收到 FIN Flags 数据包的时候就已经标记为 Closed 即链接不可写
// 但实际上两端都会再收到对端的 ACK Flags 此时如果前面的 conn 已经被删除则会被
// 认为是新创建的 conn 从而造成链接泄漏 所以应该有一个 TTL 机制用户保障此段时间
// 内链接不会被错误地 `复用`
//
// 因此需要一个 frozen 机制 不同协议可能有不同的周期 frozen 为空则代表无需此机制
type connPool struct {
	l4Proto    socket.L4Proto
	createConn CreateConnFunc
	mut        sync.RWMutex
	conns      map[socket.Tuple]Conn
	frozen     *socket.TTLCache
}

func (cp *connPool) L4Proto() socket.L4Proto {
	return cp.l4Proto
}

// NewConnPool 创建并返回一个连接池
//
// createConn 由具体的应用层协议定制并传入
// 对于 socket 和 socket.Mirror 的操作要保证原子性
func NewConnPool(l4Proto socket.L4Proto, createConn CreateConnFunc, ttl *socket.TTLCache) ConnPool {
	return &connPool{
		l4Proto:    l4Proto,
		createConn: createConn,
		conns:      make(map[socket.Tuple]Conn),
		frozen:     ttl,
	}
}

// Delete 删除一个链接实例
func (cp *connPool) Delete(st socket.Tuple) {
	cp.mut.Lock()
	defer cp.mut.Unlock()

	conn, ok := cp.conns[st]
	if !ok {
		return
	}

	conn.Free()
	delete(cp.conns, st)

	if cp.frozen != nil {
		cp.frozen.Set(st)
	}
}

// GetOrCreate 获取或者创建一个链接实例
func (cp *connPool) GetOrCreate(st socket.Tuple, serverPort socket.Port) Conn {
	if cp.frozen != nil && cp.frozen.Has(st) {
		return nil
	}

	cp.mut.RLock()
	conn := cp.getConnLocked(st)
	cp.mut.RUnlock()
	if conn != nil {
		return conn
	}

	cp.mut.Lock()
	defer cp.mut.Unlock()

	if conn := cp.getConnLocked(st); conn != nil {
		return conn
	}

	conn = cp.createConn(st, serverPort)
	cp.conns[st] = conn
	cp.conns[st.Mirror()] = conn
	return conn
}

// OnStats 触发 Stats 统计事件
func (cp *connPool) OnStats(f func(connstream.TupleStats)) {
	cp.mut.RLock()
	defer cp.mut.RUnlock()

	for _, conn := range cp.conns {
		stats := conn.Stats()
		for i := 0; i < len(stats); i++ {
			f(stats[i])
		}
	}
}

// Clean 清理资源 调用后请勿再次使用
func (cp *connPool) Clean() {
	if cp.frozen != nil {
		cp.frozen.Close()
	}

	for st, conn := range cp.conns {
		conn.Free()
		delete(cp.conns, st)
	}
}

// ActiveConns 返回活跃的 Connection 数量
func (cp *connPool) ActiveConns() int {
	return len(cp.conns)
}

// RemoveExpired 清理超过 duration 时间未有任何活跃数据包的 Conn
func (cp *connPool) RemoveExpired(duration time.Duration) {
	cp.mut.Lock()
	defer cp.mut.Unlock()

	now := time.Now()
	for st, conn := range cp.conns {
		if conn.ActiveAt().Add(duration).Before(now) {
			conn.Free()
			delete(cp.conns, st)
		}
	}
}

func (cp *connPool) getConnLocked(st socket.Tuple) Conn {
	if conn, ok := cp.conns[st]; ok {
		return conn
	}
	if conn, ok := cp.conns[st.Mirror()]; ok {
		return conn
	}
	return nil
}

type (
	// CreateRoundTripFunc 根据传入的 *role.Pair 对象创建 Data 实例
	CreateRoundTripFunc func(pair *role.Pair) socket.RoundTrip

	// CreateDecoderFunc 根据 socket.Tuple 以及 serverPort 创建 Decoder 实例
	CreateDecoderFunc func(st socket.Tuple, serverPort socket.Port) Decoder

	// CreateMatcherFunc 创建 role.Matcher 实例
	CreateMatcherFunc func() role.Matcher
)

// NewL7TCPConnPool 创建基于 TCP 协议的 Layer7 连接池
//
// 默认注册 2*MSL 的 TTL 缓存
func NewL7TCPConnPool(createMatcher CreateMatcherFunc, createRoundTrip CreateRoundTripFunc, createDecoder CreateDecoderFunc) ConnPool {
	return NewConnPool(
		socket.L4ProtoTCP,
		func(st socket.Tuple, serverPort socket.Port) Conn {
			matcher := createMatcher()
			return NewL7Conn(
				connstream.NewConn(st, connstream.NewTCPStream),
				serverPort,
				matcher,
				createRoundTrip,
				createDecoder,
			)
		},
		socket.NewTTLCache(socket.TCPMsl*2),
	)
}

// NewL7UDPConnPool 创建基于 UDP 协议的 Layer7 连接池
//
// 无需提供 TLL 缓存
func NewL7UDPConnPool(createMatcher CreateMatcherFunc, createRoundTrip CreateRoundTripFunc, createDecoder CreateDecoderFunc) ConnPool {
	return NewConnPool(
		socket.L4ProtoUDP,
		func(st socket.Tuple, serverPort socket.Port) Conn {
			matcher := createMatcher()
			return NewL7Conn(
				connstream.NewConn(st, connstream.NewUDPStream),
				serverPort,
				matcher,
				createRoundTrip,
				createDecoder,
			)
		},
		nil,
	)
}

type socketDecoder struct {
	st socket.Tuple
	d  Decoder
}

// L7TCPConn 基于 Layer4 协议的 Layer7 连接
//
// conn 内置 connstream.Conn 且根据传入的 CreateDecoderFunc 注册解析函数
type L7TCPConn struct {
	mut        sync.Mutex
	conn       *connstream.Conn
	serverPort socket.Port
	matcher    role.Matcher

	l, r *socketDecoder
	once sync.Once

	createRoundTrip CreateRoundTripFunc
	createDecoder   CreateDecoderFunc
}

// NewL7Conn 创建并返回链接实例
func NewL7Conn(conn *connstream.Conn, serverPort socket.Port, matcher role.Matcher, createRoundTrip CreateRoundTripFunc, createDecoder CreateDecoderFunc) *L7TCPConn {
	return &L7TCPConn{
		conn:            conn,
		serverPort:      serverPort,
		matcher:         matcher,
		createDecoder:   createDecoder,
		createRoundTrip: createRoundTrip,
	}
}

// IsClosed 判断链接是否已经关闭
func (c *L7TCPConn) IsClosed() bool {
	return c.conn.IsClosed()
}

// Free 释放资源
func (c *L7TCPConn) Free() {
	// 确保仅释放一次资源即可
	c.once.Do(func() {
		if c.l != nil && c.l.d != nil {
			c.l.d.Free()
		}
		if c.r != nil && c.r.d != nil {
			c.r.d.Free()
		}
	})
}

func (c *L7TCPConn) ActiveAt() time.Time {
	return c.conn.ActiveAt()
}

func (c *L7TCPConn) Stats() []connstream.TupleStats {
	return c.conn.Stats()
}

// OnL4Packet 处理 L4 数据包
func (c *L7TCPConn) OnL4Packet(pkt socket.L4Packet, ch chan<- socket.RoundTrip) error {
	c.mut.Lock()
	defer c.mut.Unlock()

	d := c.getDecoder(pkt.SocketTuple())
	err := c.conn.Write(pkt, func(r zerocopy.Reader) {
		objs, err := d.Decode(r, pkt.ArrivedTime())
		if err != nil {
			return
		}

		for i := 0; i < len(objs); i++ {
			obj := objs[i]
			if obj == nil {
				continue
			}

			pair := c.matcher.Match(obj)
			if pair == nil {
				continue
			}

			roundTrip := c.createRoundTrip(pair)
			if !roundTrip.Validate() {
				continue
			}
			ch <- roundTrip
		}
	})

	if errors.Is(err, connstream.ErrClosed) {
		return ErrConnClosed
	}
	return err
}

// getDecoder 匹配 Decoder
//
// 从 l->r 顺序匹配 会比 Map 更高效
func (c *L7TCPConn) getDecoder(st socket.Tuple) Decoder {
	if c.l != nil && c.l.st == st {
		return c.l.d
	}
	if c.r != nil && c.r.st == st {
		return c.r.d
	}

	if c.l == nil {
		c.l = &socketDecoder{st: st, d: c.createDecoder(st, c.serverPort)}
		return c.l.d
	}
	if c.r == nil {
		c.r = &socketDecoder{st: st, d: c.createDecoder(st, c.serverPort)}
		return c.r.d
	}
	return nil
}
