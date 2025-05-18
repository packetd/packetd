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
	"github.com/packetd/packetd/protocol/pamqp"
)

func init() {
	register(socket.L7ProtoAMQP, newAMQPConverter)
}

type amqpConverter struct {
	config CommonConfig
}

func newAMQPConverter(config Config) converter {
	return &amqpConverter{
		config: config.AMQP,
	}
}

func (c *amqpConverter) Proto() socket.L7Proto {
	return socket.L7ProtoAMQP
}

func (c *amqpConverter) matchLabels(req *pamqp.Request, rsp *pamqp.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	packet := req.Packet

	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.queue_name":
			lbs = append(lbs, labels.Label{Name: "queue_name", Value: packet.QueueName})
		case "request.class":
			lbs = append(lbs, labels.Label{Name: "class", Value: req.ClassMethod.Class})
		case "request.method":
			lbs = append(lbs, labels.Label{Name: "method", Value: req.ClassMethod.Method})
		}
	}
	return lbs
}

var amqpCommMetrics = commonMetrics{
	requestTotal:           "amqp_requests_total",
	requestDurationSeconds: "amqp_request_duration_seconds",
	requestBodySizeBytes:   "amqp_request_body_bytes",
	responseBodySizeBytes:  "amqp_response_body_bytes",
}

func (c *amqpConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*pamqp.Request)
	rsp := rt.Response().(*pamqp.Response)

	lbs := c.matchLabels(req, rsp)
	return generateCommonMetrics(amqpCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)
}
