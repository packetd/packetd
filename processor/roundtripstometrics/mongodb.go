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
	"fmt"

	"github.com/packetd/packetd/common/socket"
	"github.com/packetd/packetd/internal/labels"
	"github.com/packetd/packetd/internal/metricstorage"
	"github.com/packetd/packetd/protocol/pmongodb"
)

func init() {
	register(socket.L7ProtoMongoDB, newMongoDBConverter)
}

type mongodbConverter struct {
	config CommonConfig
}

func newMongoDBConverter(config Config) converter {
	return &mongodbConverter{
		config: config.MongoDB,
	}
}

func (c *mongodbConverter) Proto() socket.L7Proto {
	return socket.L7ProtoMongoDB
}

func (c *mongodbConverter) matchLabels(req *pmongodb.Request, rsp *pmongodb.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.command":
			lbs = append(lbs, labels.Label{Name: "service", Value: req.CmdName})
		case "request.source":
			lbs = append(lbs, labels.Label{Name: "source", Value: req.Source})
		case "response.ok":
			lbs = append(lbs, labels.Label{Name: "ok", Value: fmt.Sprintf("%.0f", rsp.Ok)})
		}
	}
	return lbs
}

var mangodbCommMetrics = commonMetrics{
	requestTotal:           "mongodb_requests_total",
	requestDurationSeconds: "mongodb_request_duration_seconds",
	requestBodySizeBytes:   "mongodb_request_body_bytes",
	responseBodySizeBytes:  "mongodb_response_body_bytes",
}

func (c *mongodbConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*pmongodb.Request)
	rsp := rt.Response().(*pmongodb.Response)

	lbs := c.matchLabels(req, rsp)
	return generateCommonMetrics(mangodbCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)
}
