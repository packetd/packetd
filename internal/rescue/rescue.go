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

package rescue

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/logger"
)

var panicTotal = promauto.NewCounter(
	prometheus.CounterOpts{
		Namespace: common.App,
		Name:      "panic_total",
		Help:      "program causes panic total",
	},
)

var PanicHandlers = []func(any){
	incPanicCounter,
	logPanic,
}

func incPanicCounter(_ any) {
	panicTotal.Inc()
}

func logPanic(r any) {
	const size = 64 << 10
	stacktrace := make([]byte, size)
	stacktrace = stacktrace[:runtime.Stack(stacktrace, false)]
	if _, ok := r.(string); ok {
		logger.Errorf("Observed a panic: %s\n%s", r, stacktrace)
	} else {
		logger.Errorf("Observed a panic: %#v (%v)\n%s", r, r, stacktrace)
	}
}

func HandleCrash() {
	if r := recover(); r != nil {
		for _, fn := range PanicHandlers {
			fn(r)
		}
	}
}
