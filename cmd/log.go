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

	"github.com/packetd/packetd/common"
	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/controller"
	"github.com/packetd/packetd/internal/sigs"
)

type logCmdConfig struct {
	Console          bool
	File             string
	Ifaces           string
	IPv4Only         bool
	NoPromiscuous    bool
	RoundtripFile    string
	RoundtripSize    int
	RoundtripBackups int
	Protocols        []string
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
  stdout: true

sniffer:
  ifaces: {{ .Ifaces }}
  file: {{ .File }}
  ipv4Only: {{ .IPv4Only }}
  noPromiscuous: {{ .NoPromiscuous }}
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
    console: {{ .Console }}
    filename: {{ .RoundtripFile }}
    maxSize: {{ .RoundtripSize }}
    maxBackups: {{ .RoundtripBackups }}
    maxAge: 7
`
	tpl, err := template.New("Config").Parse(text)
	if err != nil {
		return nil
	}

	var buf bytes.Buffer
	err = tpl.Execute(&buf, map[string]interface{}{
		"File":             c.File,
		"Console":          c.Console,
		"Ifaces":           c.Ifaces,
		"IPv4Only":         c.IPv4Only,
		"Protos":           c.decodeProtoConfig(),
		"RoundtripFile":    c.RoundtripFile,
		"RoundtripSize":    c.RoundtripSize,
		"RoundtripBackups": c.RoundtripBackups,
	})
	if err != nil {
		return nil
	}
	return buf.Bytes()
}

var logConfig logCmdConfig

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Capture and log network traffic roundtrip",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := confengine.LoadContent(logConfig.Yaml())
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
			os.Exit(1)
		}

		ctr, err := controller.New(cfg, common.BuildInfo{
			Version: version,
			GitHash: gitHash,
			Time:    buildTime,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create controller: %v\n"+
				"Note: This operation may requires root privileges (try running with 'sudo')", err)
			os.Exit(1)
		}
		if err := ctr.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to start controller: %v\n", err)
			os.Exit(1)
		}

		<-sigs.Terminate()
		ctr.Stop()
	},
	Example: "# packetd log --proto 'http;80,8080' --proto 'dns;53' --ifaces any --console",
}

func init() {
	logCmd.Flags().BoolVar(&logConfig.Console, "console", false, "Enable console logging")
	logCmd.Flags().BoolVar(&logConfig.NoPromiscuous, "no-promiscuous", false, "Don't put the interface into promiscuous mode")
	logCmd.Flags().StringVar(&logConfig.File, "pcap.file", "", "Path to pcap file to read from")
	logCmd.Flags().StringVar(&logConfig.Ifaces, "ifaces", "any", "Network interfaces to monitor (supports regex), 'any' for all interfaces")
	logCmd.Flags().StringSliceVar(&logConfig.Protocols, "proto", nil, "Protocols to capture in 'protocol;ports[;host]' format, multiple protocols supported")
	logCmd.Flags().BoolVar(&logConfig.IPv4Only, "ipv4", false, "Capture IPv4 traffic only")
	logCmd.Flags().StringVar(&logConfig.RoundtripFile, "roundtrip.file", "packetd.roundtrip", "Path to roundtrip file")
	logCmd.Flags().IntVar(&logConfig.RoundtripSize, "roundtrip.size", 100, "Maximum size of roundtrip file in MB")
	logCmd.Flags().IntVar(&logConfig.RoundtripBackups, "roundtrip.backups", 10, "Maximum number of old roundtrip files to retain")
	rootCmd.AddCommand(logCmd)
}
