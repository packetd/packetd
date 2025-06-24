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
	"github.com/packetd/packetd/protocol/pkafka"
)

func init() {
	register(socket.L7ProtoKafka, newKafkaConverter)
}

type kafkaConverter struct {
	config CommonConfig
}

func newKafkaConverter(config Config) converter {
	return &kafkaConverter{
		config: config.Kafka,
	}
}

func (c *kafkaConverter) Proto() socket.L7Proto {
	return socket.L7ProtoKafka
}

func (c *kafkaConverter) matchLabels(req *pkafka.Request, rsp *pkafka.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	packet := req.Packet
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.api":
			lbs = append(lbs, labels.Label{Name: "api", Value: packet.API})
		case "request.version":
			lbs = append(lbs, labels.Label{Name: "version", Value: strconv.Itoa(int(packet.APIVersion))})
		}
	}
	return lbs
}

var kafkaCommMetrics = commonMetrics{
	requestTotal:           "kafka_requests_total",
	requestDurationSeconds: "kafka_request_duration_seconds",
	requestBodySizeBytes:   "kafka_request_body_bytes",
	responseBodySizeBytes:  "kafka_response_body_bytes",
}

func (c *kafkaConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*pkafka.Request)
	rsp := rt.Response().(*pkafka.Response)

	lbs := c.matchLabels(req, rsp)
	return generateCommonMetrics(kafkaCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)
}
