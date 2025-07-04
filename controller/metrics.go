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

package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/packetd/packetd/common"
)

var (
	uptime = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: common.App,
			Name:      "uptime",
			Help:      "Uptime in seconds",
		},
	)

	buildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: common.App,
			Name:      "build_info",
			Help:      "Build information",
		},
		[]string{"version", "git_hash", "build_time"},
	)

	snifferReceivedPackets = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: common.App,
			Name:      "sniffer_received_packets_total",
			Help:      "Sniffer received packets total",
		},
		[]string{"iface"},
	)

	snifferDroppedPackets = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: common.App,
			Name:      "sniffer_dropped_packets_total",
			Help:      "Sniffer dropped packets total",
		},
		[]string{"iface"},
	)

	handledRoundtrips = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: common.App,
			Name:      "handled_roundtrips_total",
			Help:      "Handled roundtrips total",
		},
	)
)
