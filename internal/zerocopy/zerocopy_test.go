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

package zerocopy

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/common/socket"
)

func TestZeroCopy(t *testing.T) {
	t.Run("Read", func(t *testing.T) {
		n := 64
		buf := NewBuffer(bytes.Repeat([]byte("a"), n*common.ReadWriteBlockSize))

		for i := 0; i < n; i++ {
			_, err := buf.Read(common.ReadWriteBlockSize)
			assert.NoError(t, err)
		}
		_, err := buf.Read(1)
		assert.Equal(t, io.EOF, err)
	})

	t.Run("Close", func(t *testing.T) {
		buf := NewBuffer(bytes.Repeat([]byte("a"), 1024))
		buf.Close()
		_, err := buf.Read(1)
		assert.Equal(t, io.EOF, err)
	})
}

func BenchmarkZeroCopyBuffer(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := NewBuffer(nil)
			buf.Write(bytes.Repeat([]byte("a"), socket.MaxIPV4PacketSize))
			for {
				data, err := buf.Read(common.ReadWriteBlockSize)
				if err != nil {
					break
				}
				_ = data // 避免编译器优化
			}
		}
	})
}

func BenchmarkBuffer(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := bytes.NewBuffer(nil)
			buf.Write(bytes.Repeat([]byte("a"), socket.MaxIPV4PacketSize))
			for {
				data := make([]byte, common.ReadWriteBlockSize)
				_, err := buf.Read(data)
				if err != nil {
					break
				}
			}
		}
	})
}
