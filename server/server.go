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

package server

import (
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/gorilla/mux"

	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/logger"
)

type Config struct {
	Enabled bool          `config:"enabled"`
	Address string        `config:"address"`
	Pprof   bool          `config:"pprof"`
	Timeout time.Duration `config:"timeout"`
}

type Server struct {
	config Config
	router *mux.Router
	server *http.Server
}

// New 创建并返回 Server 实例
//
// 当 .Enabled 为 false 时会返回空指针 调用方需先判断
func New(conf *confengine.Config) (*Server, error) {
	var config Config
	if err := conf.UnpackChild("server", &config); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, nil
	}

	router := mux.NewRouter()
	s := &Server{
		config: config,
		router: router,
		server: &http.Server{
			Handler:      router,
			ReadTimeout:  config.Timeout,
			WriteTimeout: config.Timeout,
		},
	}
	if config.Pprof {
		s.registerPprofRoutes()
	}
	return s, nil
}

func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		return err
	}
	logger.Infof("server listening on %s", s.config.Address)
	return s.server.Serve(l)
}

func (s *Server) RegisterGetRoute(path string, f http.HandlerFunc) {
	s.router.Methods(http.MethodGet).Path(path).HandlerFunc(f)
}

func (s *Server) RegisterPostRoute(path string, f http.HandlerFunc) {
	s.router.Methods(http.MethodPost).Path(path).HandlerFunc(f)
}

func (s *Server) registerPprofRoutes() {
	s.RegisterGetRoute("/debug/pprof/cmdline", pprof.Cmdline)
	s.RegisterGetRoute("/debug/pprof/profile", pprof.Profile)
	s.RegisterGetRoute("/debug/pprof/symbol", pprof.Symbol)
	s.RegisterGetRoute("/debug/pprof/trace", pprof.Trace)
	s.RegisterGetRoute("/debug/pprof/{other}", pprof.Index)
}
