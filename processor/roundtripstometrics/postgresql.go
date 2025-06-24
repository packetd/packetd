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
	"github.com/packetd/packetd/protocol/ppostgresql"
)

func init() {
	register(socket.L7ProtoPostgreSQL, newPostgreSQLConverter)
}

type postgresqlConverter struct {
	config CommonConfig
}

func newPostgreSQLConverter(config Config) converter {
	return &postgresqlConverter{
		config: config.PostgreSQL,
	}
}

func (c *postgresqlConverter) Proto() socket.L7Proto {
	return socket.L7ProtoPostgreSQL
}

func (c *postgresqlConverter) matchLabels(req *ppostgresql.Request, rsp *ppostgresql.Response) labels.Labels {
	var name string
	namer, ok := req.Packet.(interface{ Name() string })
	if ok {
		name = namer.Name()
	}

	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.command":
			lbs = append(lbs, labels.Label{Name: "command", Value: name})

		case "response.packet_type":
			var packetType string
			if obj, ok := rsp.Packet.(interface{ Name() string }); ok {
				packetType = obj.Name()
			}
			lbs = append(lbs, labels.Label{Name: "packet_type", Value: packetType})
		}
	}
	return lbs
}

var postgresqlCommMetrics = commonMetrics{
	requestTotal:           "postgresql_request_total",
	requestDurationSeconds: "postgresql_request_duration_seconds",
	requestBodySizeBytes:   "postgresql_request_body_size_bytes",
	responseBodySizeBytes:  "postgresql_response_body_size_bytes",
}

func (c *postgresqlConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*ppostgresql.Request)
	rsp := rt.Response().(*ppostgresql.Response)

	lbs := c.matchLabels(req, rsp)
	metrics := generateCommonMetrics(postgresqlCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)

	switch packet := rsp.Packet.(type) {
	case *ppostgresql.CommandCompletePacket:
		metrics = append(metrics, metricstorage.ConstMetric{
			Name:   "postgresql_response_affected_rows",
			Model:  metricstorage.ModelCounter,
			Labels: lbs,
			Value:  float64(packet.Rows),
		})
	}

	return metrics
}
