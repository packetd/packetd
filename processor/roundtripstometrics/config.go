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
	"time"

	"github.com/packetd/packetd/internal/labels"
	"github.com/packetd/packetd/internal/metricstorage"
)

type CommonConfig struct {
	RequireLabels []string `config:"requireLabels" mapstructure:"requireLabels"`
}

type Config struct {
	Expired    time.Duration `config:"expired" mapstructure:"expired"`
	HTTP       CommonConfig  `config:"http" mapstructure:"http"`
	Redis      CommonConfig  `config:"redis" mapstructure:"redis"`
	MySQL      CommonConfig  `config:"mysql" mapstructure:"mysql"`
	HTTP2      CommonConfig  `config:"http2" mapstructure:"http2"`
	GRPC       CommonConfig  `config:"grpc" mapstructure:"grpc"`
	DNS        CommonConfig  `config:"dns" mapstructure:"dns"`
	MongoDB    CommonConfig  `config:"mongodb" mapstructure:"mongodb"`
	PostgreSQL CommonConfig  `config:"postgresql" mapstructure:"postgresql"`
	Kafka      CommonConfig  `config:"kafka" mapstructure:"kafka"`
	AMQP       CommonConfig  `config:"amqp" mapstructure:"amqp"`
}

func matchCommonLabels(required []string, src, dst string, sport, dport uint16) labels.Labels {
	var lbs labels.Labels
	for _, label := range required {
		switch label {
		case "client.address":
			lbs = append(lbs, labels.Label{Name: "client_address", Value: src})
		case "client.port":
			lbs = append(lbs, labels.Label{Name: "client_port", Value: strconv.Itoa(int(sport))})
		case "server.address":
			lbs = append(lbs, labels.Label{Name: "server_address", Value: dst})
		case "server.port":
			lbs = append(lbs, labels.Label{Name: "server_port", Value: strconv.Itoa(int(dport))})
		}
	}
	return lbs
}

type commonMetrics struct {
	requestTotal           string
	requestDurationSeconds string
	requestBodySizeBytes   string
	responseBodySizeBytes  string
}

func generateCommonMetrics(cm commonMetrics, lbs labels.Labels, secs float64, reqSize, rspSize int) []metricstorage.ConstMetric {
	return []metricstorage.ConstMetric{
		metricstorage.NewCounterConstMetric(cm.requestTotal, 1, lbs),
		metricstorage.NewHistogramConstMetric(cm.requestDurationSeconds, secs, metricstorage.UnitSeconds, lbs),
		metricstorage.NewHistogramConstMetric(cm.requestBodySizeBytes, float64(reqSize), metricstorage.UnitBytes, lbs),
		metricstorage.NewHistogramConstMetric(cm.responseBodySizeBytes, float64(rspSize), metricstorage.UnitBytes, lbs),
	}
}
