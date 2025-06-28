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
	"github.com/packetd/packetd/protocol/predis"
)

func init() {
	register(socket.L7ProtoRedis, newRedisConverter)
}

type redisConverter struct {
	config CommonConfig
}

func newRedisConverter(config Config) converter {
	return &redisConverter{
		config: config.Redis,
	}
}

func (c *redisConverter) Proto() socket.L7Proto {
	return socket.L7ProtoRedis
}

func (c *redisConverter) matchLabels(req *predis.Request, rsp *predis.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.command":
			lbs = append(lbs, labels.Label{Name: "command", Value: req.Command})
		case "response.data_type":
			lbs = append(lbs, labels.Label{Name: "data_type", Value: rsp.DataType})
		}
	}
	return lbs
}

var redisCommMetrics = commonMetrics{
	requestTotal:           "redis_requests_total",
	requestDurationSeconds: "redis_request_duration_seconds",
	requestBodySizeBytes:   "redis_request_body_size_bytes",
	responseBodySizeBytes:  "redis_response_body_size_bytes",
}

func (c *redisConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*predis.Request)
	rsp := rt.Response().(*predis.Response)

	lbs := c.matchLabels(req, rsp)
	return generateCommonMetrics(redisCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)
}
