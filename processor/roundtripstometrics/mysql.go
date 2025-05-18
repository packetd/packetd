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
	"github.com/packetd/packetd/protocol/pmysql"
)

func init() {
	register(socket.L7ProtoMySQL, newMySQLConverter)
}

type mysqlConverter struct {
	config CommonConfig
}

func newMySQLConverter(config Config) converter {
	return &mysqlConverter{
		config: config.MySQL,
	}
}

func (c *mysqlConverter) Proto() socket.L7Proto {
	return socket.L7ProtoMySQL
}

func (c *mysqlConverter) matchLabels(req *pmysql.Request, rsp *pmysql.Response) labels.Labels {
	lbs := matchCommonLabels(c.config.RequireLabels, req.Host, rsp.Host, req.Port, rsp.Port)
	for _, label := range c.config.RequireLabels {
		switch label {
		case "request.command":
			lbs = append(lbs, labels.Label{Name: "method", Value: req.Command})

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

var mysqlCommMetrics = commonMetrics{
	requestTotal:           "mysql_request_total",
	requestDurationSeconds: "mysql_request_duration_seconds",
	requestBodySizeBytes:   "mysql_request_body_size_bytes",
	responseBodySizeBytes:  "mysql_response_body_size_bytes",
}

func (c *mysqlConverter) Convert(rt socket.RoundTrip) []metricstorage.ConstMetric {
	req := rt.Request().(*pmysql.Request)
	rsp := rt.Response().(*pmysql.Response)

	lbs := c.matchLabels(req, rsp)
	metrics := generateCommonMetrics(mysqlCommMetrics, lbs, rt.Duration().Seconds(), req.Size, rsp.Size)

	switch packet := rsp.Packet.(type) {
	case *pmysql.OKPacket:
		metrics = append(metrics, metricstorage.ConstMetric{
			Name:   "mysql_response_affected_rows",
			Model:  metricstorage.ModelCounter,
			Labels: lbs,
			Value:  float64(packet.AffectedRows),
		})

	case *pmysql.ResultSetPacket:
		metrics = append(metrics, metricstorage.ConstMetric{
			Name:   "mysql_response_resultset_rows",
			Model:  metricstorage.ModelHistogram,
			Labels: lbs,
			Unit:   metricstorage.UnitBytes,
			Value:  float64(packet.Rows),
		})
	}

	return metrics
}
