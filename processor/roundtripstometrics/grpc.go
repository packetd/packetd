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
	"github.com/packetd/packetd/protocol/pgrpc"
)

func init() {
	register(socket.L7ProtoGRPC, newGRPCConverter)
}

type grpcConverter struct {
	config CommonConfig
}

func newGRPCConverter(config Config) converter {
	return &grpcConverter{
		config: config.GRPC,
	}
}

func (c *grpcConverter) Proto() socket.L7Proto {
	return socket.L7ProtoGRPC
}

func (c *grpcConverter) matchLabels(req *pgrpc.Request, rsp *pgrpc.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.service":
			lbs = append(lbs, labels.Label{Name: "method", Value: req.Service})
		case "response.status_code":
			lbs = append(lbs, labels.Label{Name: "status_code", Value: rsp.Status})
		}
	}
	return lbs
}

var grpcCommMetrics = commonMetrics{
	requestTotal:           "grpc_requests_total",
	requestDurationSeconds: "grpc_request_duration_seconds",
	requestBodySizeBytes:   "grpc_request_body_bytes",
	responseBodySizeBytes:  "grpc_response_body_bytes",
}

func (c *grpcConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*pgrpc.Request)
	rsp := rt.Response().(*pgrpc.Response)

	lbs := c.matchLabels(req, rsp)
	return generateCommonMetrics(grpcCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)
}
