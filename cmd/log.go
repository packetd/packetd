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

package cmd

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/controller"
	"github.com/packetd/packetd/internal/sigs"
)

type logCmdConfig struct {
	File       string
	Ifaces     string
	IPv4Only   bool
	LogFile    string
	LogSize    int
	LogBackups int
	Protocols  []string
}

type protoConfig struct {
	Name     string
	Protocol string
	Ports    []int
	Host     string
}

func (c *logCmdConfig) decodeProtoConfig() []protoConfig {
	var pcs []protoConfig
	for idx, proto := range c.Protocols {
		parts := strings.Split(proto, ";")
		if len(parts) < 2 {
			continue
		}

		var pc protoConfig
		for _, port := range strings.Split(parts[1], ",") {
			i, err := strconv.Atoi(port)
			if err != nil {
				continue
			}
			pc.Ports = append(pc.Ports, i)
		}

		pc.Name = strconv.Itoa(idx)
		pc.Protocol = parts[0]

		if len(parts) > 2 {
			pc.Host = parts[2]
		}
		pcs = append(pcs, pc)
	}
	return pcs
}

func (c *logCmdConfig) Yaml() []byte {
	text := `
controller:
processor:
pipeline:
metricsStorage:
server:
logger:

sniffer:
  ifaces: {{ .Ifaces }}
  file: {{ .File }}
  ipv4Only: {{ .IPv4Only }}
  protocols:
    rules:
{{ range .Protos }}
    - name: {{ .Name }}
      protocol: {{ .Protocol }}
      ports: {{ .Ports }}
      host: {{ .Host }}
{{ end }}

exporter:
  metrics:
  traces:
  roundtrips:
    enabled: true
    console: false
    filename: {{ .LogFile }}
    maxSize: {{ .LogSize }}
    maxBackups: {{ .LogBackups }}
    maxAge: 30
`
	tpl, err := template.New("Config").Parse(text)
	if err != nil {
		return nil
	}

	var buf bytes.Buffer
	err = tpl.Execute(&buf, map[string]interface{}{
		"File":       c.File,
		"Ifaces":     c.Ifaces,
		"IPv4Only":   c.IPv4Only,
		"Protos":     c.decodeProtoConfig(),
		"LogFile":    c.LogFile,
		"LogSize":    c.LogSize,
		"LogBackups": c.LogBackups,
	})
	if err != nil {
		return nil
	}
	return buf.Bytes()
}

var logConfig logCmdConfig

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Capture and record network traffic in roundtrips mode",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := confengine.LoadContent(logConfig.Yaml())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
			os.Exit(1)
		}

		ctr, err := controller.New(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create controller: %v\n", err)
			os.Exit(1)
		}
		if err := ctr.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to start controller: %v\n", err)
			os.Exit(1)
		}

		<-sigs.Terminate()
		ctr.Stop()
	},
	Example: "# packetd log --proto http;80,8080; --proto dns;53 --iface any --log.file roundtrips.log",
}

func init() {
	logCmd.Flags().StringVar(&logConfig.File, "pcap.file", "", "Path to pcap file to read from")
	logCmd.Flags().StringVar(&logConfig.Ifaces, "ifaces", "any", "Network interfaces to monitor (supports regex), 'any' for all interfaces")
	logCmd.Flags().StringSliceVar(&logConfig.Protocols, "proto", nil, "Protocols to capture in 'protocol;ports[;host]' format, multiple protocols supported")
	logCmd.Flags().BoolVar(&logConfig.IPv4Only, "ipv4", false, "Capture IPv4 traffic only")
	logCmd.Flags().StringVar(&logConfig.LogFile, "log.file", "roundtrips.log", "Path to log file")
	logCmd.Flags().IntVar(&logConfig.LogSize, "log.size", 100, "Maximum size of log file in MB")
	logCmd.Flags().IntVar(&logConfig.LogBackups, "log.backups", 10, "Maximum number of old log files to retain")
	rootCmd.AddCommand(logCmd)
}
