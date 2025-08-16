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
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/packetd/packetd/connstream"
	"github.com/packetd/packetd/internal/sigs"
	"github.com/packetd/packetd/logger"
)

func (c *Controller) setupServer() {
	if c.svr == nil {
		return
	}

	// Admin Routes
	c.svr.RegisterPostRoute("/-/logger", c.routeLogger)
	c.svr.RegisterPostRoute("/-/reload", c.recordReload)

	// Watch Routes
	c.svr.RegisterGetRoute("/watch", c.routeWatch)

	// Metrics Routes
	c.svr.RegisterGetRoute("/metrics", c.routeMetrics)
	c.svr.RegisterGetRoute("/protocol/metrics", c.routeProtoMetrics)
}

func (c *Controller) routeMetrics(w http.ResponseWriter, r *http.Request) {
	c.recordMetrics()
	promhttp.Handler().ServeHTTP(w, r)
}

func (c *Controller) routeProtoMetrics(w http.ResponseWriter, r *http.Request) {
	if c.storage == nil {
		return
	}
	c.pps.RangePoolStats(func(stats connstream.TupleStats) {
		c.updatePoolStats(stats)
	})
	c.updateActivePoolConns(c.pps.ActivePoolConns())
	c.storage.WritePrometheus(w)
}

func (c *Controller) routeLogger(w http.ResponseWriter, r *http.Request) {
	level := r.FormValue("level")
	logger.SetLoggerLevel(level)
	w.Write([]byte(`{"status": "success"}`))
}

func (c *Controller) recordReload(w http.ResponseWriter, r *http.Request) {
	if err := sigs.SelfReload(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

func (c *Controller) routeWatch(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	var maxMessage int
	maxMessage, _ = strconv.Atoi(r.URL.Query().Get("max_message"))
	if maxMessage <= 0 {
		maxMessage = 100
	}

	var timeout time.Duration
	timeout, _ = time.ParseDuration(r.URL.Query().Get("timeout"))
	if timeout <= 0 {
		timeout = time.Second * 5
	}

	queue := c.rtBus.Subscribe(10)
	defer c.rtBus.Unsubscribe(queue)

	for i := 0; i < maxMessage; i++ {
		data, ok := queue.PopTimeout(timeout)
		if !ok {
			return
		}

		w.Write(data.([]byte))
		w.Write([]byte{'\n'})
		flusher.Flush()
	}
}
