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
	"github.com/packetd/packetd/protocol/pdns"
)

func init() {
	register(socket.L7ProtoDNS, newDNSConverter)
}

type dnsConverter struct {
	config CommonConfig
}

func newDNSConverter(config Config) converter {
	return &dnsConverter{
		config: config.DNS,
	}
}

func (c *dnsConverter) Proto() socket.L7Proto {
	return socket.L7ProtoDNS
}

func (c *dnsConverter) matchLabels(req *pdns.Request, rsp *pdns.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.question":
			lbs = append(lbs, labels.Label{Name: "question", Value: req.Message.QuestionSec.Name})
		}
	}
	return lbs
}

var dnsCommMetrics = commonMetrics{
	requestTotal:           "dns_requests_total",
	requestDurationSeconds: "dns_request_duration_seconds",
	requestBodySizeBytes:   "dns_request_body_bytes",
	responseBodySizeBytes:  "dns_response_body_bytes",
}

func (c *dnsConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*pdns.Request)
	rsp := rt.Response().(*pdns.Response)

	lbs := c.matchLabels(req, rsp)
	return generateCommonMetrics(dnsCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)
}
