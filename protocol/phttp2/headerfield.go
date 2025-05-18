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

package phttp2

import (
	"net/http"

	fasthttp2 "github.com/dgrr/http2"
)

// HeaderField Header Field 为 HTTP/2 中的 header 实体
type HeaderField struct {
	Name  string
	Value string
}

// HeaderFields 封装了一组 Fields 此方法线程不安全 不允许并发调用
type HeaderFields struct {
	fields      map[string]string
	trailerKeys []string
}

// NewHeaderFields 创建并返回 *HeaderFields 实例
func NewHeaderFields(trailerKeys ...string) *HeaderFields {
	return &HeaderFields{
		trailerKeys: trailerKeys,
		fields:      make(map[string]string),
	}
}

// Set 更新 Field 同名覆盖
func (hfs *HeaderFields) Set(field HeaderField) {
	hfs.fields[field.Name] = field.Value
}

// Get 根据 name 获取 Field
func (hfs *HeaderFields) Get(name string) (HeaderField, bool) {
	v, ok := hfs.fields[name]
	if !ok {
		return HeaderField{}, false
	}

	return HeaderField{
		Name:  name,
		Value: v,
	}, true
}

// Exist 返回 name 是否存在
func (hfs *HeaderFields) Exist(name string) bool {
	_, ok := hfs.fields[name]
	return ok
}

// RequestField HTTP/2 Request pseudo header 转换为 HTTP2 协议属性
type RequestField struct {
	Method    string
	Scheme    string
	Path      string
	Authority string
}

var pseudoHeaders = map[string]struct{}{
	headerMethod:    {},
	headerScheme:    {},
	headerPath:      {},
	headerAuthority: {},
	headerStatus:    {},
}

// RequestHeader 返回 Request pseudo header 以及剩余属性
func (hfs *HeaderFields) RequestHeader() (RequestField, http.Header) {
	header := make(http.Header)
	for k, v := range hfs.fields {
		_, ok := pseudoHeaders[k]
		if ok {
			continue
		}
		header.Set(k, v)
	}

	field := RequestField{
		Method:    hfs.fields[headerMethod],
		Scheme:    hfs.fields[headerScheme],
		Path:      hfs.fields[headerPath],
		Authority: hfs.fields[headerAuthority],
	}
	return field, header
}

// ResponseField HTTP/2 Response pseudo header 转换为 HTTP2 协议属性
type ResponseField struct {
	Status string
}

// ResponseHeader 返回 Response pseudo header 以及剩余属性
func (hfs *HeaderFields) ResponseHeader() (ResponseField, http.Header) {
	header := make(http.Header)
	for k, v := range hfs.fields {
		_, ok := pseudoHeaders[k]
		if ok {
			continue
		}
		header.Set(k, v)
	}

	field := ResponseField{
		Status: hfs.fields[headerStatus],
	}
	return field, header
}

// IsTrailers 判断是否为 Trailers Header
func (hfs *HeaderFields) IsTrailers() bool {
	if hfs == nil || len(hfs.trailerKeys) == 0 {
		return false
	}

	for i := 0; i < len(hfs.trailerKeys); i++ {
		if !hfs.Exist(hfs.trailerKeys[i]) {
			return false
		}
	}
	return true
}

// HeaderFieldDecoder HeaderField 解析器 单个 TCP 链接唯一 链接中的所有 Stream 共享
//
// HTTP/2 引入 HPACK 压缩算法 显著减少 Header 传输的数据量 HPACK 特性如下
//
// * 静态表 (Static Table): 预定义常见头部键值对 避免重复传输高频字段
// * 动态表 (Dynamic Table): 缓存链接中的动态键值对 动态表大小有限 遵循先进先出（FIFO）策略
// * 霍夫曼编码 (Huffman Coding): 对头部值进行高效的压缩编码 进一步减少体积
//
// TODO(mando): 这里的设计是有缺陷的
//   - 无法获取【已经建链】的会话的前面已经发送过的 Header Field
//     这部分数据只存在客户端或者服务端程序的内存中 即拿到了 Index 也无法从 Table 中构建出正确的 Field
type HeaderFieldDecoder struct {
	trailerKeys []string
	decoder     *fasthttp2.HPACK
}

// NewHeaderFieldDecoder 构建并返回 HeaderFieldDecoder 实例
//
// 初始化是向 *Hpack Pool 申请了实例 需在销毁时调用 Release 释放资源
func NewHeaderFieldDecoder(trailerKeys ...string) *HeaderFieldDecoder {
	return &HeaderFieldDecoder{
		trailerKeys: trailerKeys,
		decoder:     fasthttp2.AcquireHPACK(),
	}
}

// Decode 解析 HeaderFields
//
// 调用方必须保证传入完整的 Header 数据包 否则解析会出错
func (hfd *HeaderFieldDecoder) Decode(b []byte) *HeaderFields {
	var err error
	buf := b

	headerFields := NewHeaderFields(hfd.trailerKeys...)
	field := &fasthttp2.HeaderField{}
	for len(buf) > 0 {
		field.Reset()
		buf, err = hfd.decoder.Next(field, buf)
		if err != nil {
			break // 一旦出现未找到的 header 则退出
		}

		if field.Key() == "" {
			continue
		}
		headerFields.Set(HeaderField{
			Name:  field.Key(),
			Value: field.Value(),
		})
	}
	return headerFields
}

// Release 释放 *Hpack 资源
func (hfd *HeaderFieldDecoder) Release() {
	hfd.decoder.Reset()
	fasthttp2.ReleaseHPACK(hfd.decoder)
}
