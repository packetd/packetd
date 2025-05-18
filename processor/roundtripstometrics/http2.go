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

package roundtripstometrics

import (
	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/labels"
	"github.com/packetd/packetd/internal/metricstorage"
	"github.com/packetd/packetd/protocol/phttp2"
)

func init() {
	register(socket.L7ProtoHTTP2, newHTTP2Converter)
}

type http2Converter struct {
	config CommonConfig
}

func newHTTP2Converter(config Config) converter {
	return &http2Converter{
		config: config.HTTP2,
	}
}

func (c *http2Converter) Proto() socket.L7Proto {
	return socket.L7ProtoHTTP2
}

func (c *http2Converter) matchLabels(req *phttp2.Request, rsp *phttp2.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.method":
			lbs = append(lbs, labels.Label{Name: "method", Value: req.Method})
		case "request.path":
			lbs = append(lbs, labels.Label{Name: "path", Value: req.Path})
		case "response.status_code":
			lbs = append(lbs, labels.Label{Name: "status_code", Value: rsp.Status})
		}
	}
	return lbs
}

var http2CommMetrics = commonMetrics{
	requestTotal:           "http2_requests_total",
	requestDurationSeconds: "http2_request_duration_seconds",
	requestBodySizeBytes:   "http2_request_body_size_bytes",
	responseBodySizeBytes:  "http2_response_body_size_bytes",
}

func (c *http2Converter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*phttp2.Request)
	rsp := rt.Response().(*phttp2.Response)

	lbs := c.matchLabels(req, rsp)
	return generateCommonMetrics(http2CommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)
}
