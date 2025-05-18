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
	_ "github.com/packetd/packetd/exporter/sinker/metrics"
	_ "github.com/packetd/packetd/exporter/sinker/roundtrips"
	_ "github.com/packetd/packetd/exporter/sinker/traces"
	_ "github.com/packetd/packetd/processor/roundtripstometrics"
	_ "github.com/packetd/packetd/processor/roundtripstotraces"
	_ "github.com/packetd/packetd/protocol/pamqp"
	_ "github.com/packetd/packetd/protocol/pdns"
	_ "github.com/packetd/packetd/protocol/pgrpc"
	_ "github.com/packetd/packetd/protocol/phttp"
	_ "github.com/packetd/packetd/protocol/phttp2"
	_ "github.com/packetd/packetd/protocol/pkafka"
	_ "github.com/packetd/packetd/protocol/pmongodb"
	_ "github.com/packetd/packetd/protocol/pmysql"
	_ "github.com/packetd/packetd/protocol/ppostgresql"
	_ "github.com/packetd/packetd/protocol/predis"
	_ "github.com/packetd/packetd/sniffer/libpcap"
)
