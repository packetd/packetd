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

package metrics

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/exporter"
	"github.com/packetd/packetd/logger"
)

func init() {
	exporter.Register(common.RecordMetrics, New)
}

type Sinker struct {
	ctx    context.Context
	cancel context.CancelFunc

	cli *http.Client
	cfg *exporter.MetricsConfig
}

func New(conf exporter.Config) (exporter.Sinker, error) {
	cfg := &conf.Metrics
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	cli := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Sinker{
		ctx:    ctx,
		cancel: cancel,
		cfg:    cfg,
		cli:    cli,
	}, nil
}

func (s *Sinker) Name() common.RecordType {
	return common.RecordMetrics
}

func (s *Sinker) Sink(data any) error {
	wr, ok := data.(proto.Message)
	if !ok {
		return nil
	}

	b, err := proto.Marshal(wr)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(s.ctx, s.cfg.Timeout)
	defer cancel()

	compressed := snappy.Encode(nil, b)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, bytes.NewBuffer(compressed))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	for k, v := range s.cfg.Header {
		req.Header.Add(k, v)
	}

	rsp, err := s.cli.Do(req)
	if err != nil {
		return err
	}

	if rsp.StatusCode >= 400 && rsp.StatusCode < 500 {
		logger.Warnf("failed to sink metrics, status_code: %d", rsp.StatusCode)
	}

	io.Copy(io.Discard, rsp.Body)
	defer rsp.Body.Close()
	return nil
}

func (s *Sinker) Close() {
	s.cancel()
}
