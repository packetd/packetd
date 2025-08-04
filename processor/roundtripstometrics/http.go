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
	"strconv"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/labels"
	"github.com/packetd/packetd/internal/metricstorage"
	"github.com/packetd/packetd/protocol/phttp"
)

func init() {
	register(socket.L7ProtoHTTP, newHTTPConverter)
}

type httpConverter struct {
	config CommonConfig
}

func newHTTPConverter(config Config) converter {
	return &httpConverter{
		config: config.HTTP,
	}
}

func (c *httpConverter) Proto() socket.L7Proto {
	return socket.L7ProtoHTTP
}

func (c *httpConverter) matchLabels(req *phttp.Request, rsp *phttp.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.method":
			lbs = append(lbs, labels.Label{Name: "method", Value: req.Method})
		case "request.path":
			lbs = append(lbs, labels.Label{Name: "path", Value: req.Path})
		case "request.remote_host":
			lbs = append(lbs, labels.Label{Name: "remote_host", Value: req.RemoteHost})
		case "response.status_code":
			lbs = append(lbs, labels.Label{Name: "status_code", Value: strconv.Itoa(rsp.StatusCode)})
		}
	}
	return lbs
}

var httpCommMetrics = commonMetrics{
	requestTotal:           "http_requests_total",
	requestDurationSeconds: "http_request_duration_seconds",
	requestBodySizeBytes:   "http_request_body_bytes",
	responseBodySizeBytes:  "http_response_body_bytes",
}

func (c *httpConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*phttp.Request)
	rsp := rt.Response().(*phttp.Response)

	lbs := c.matchLabels(req, rsp)
	return generateCommonMetrics(httpCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)
}
