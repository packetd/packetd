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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/controller"
	"github.com/packetd/packetd/internal/sigs"
)

type watchCmdConfig struct {
	Console          bool
	File             string
	Ifaces           string
	IPVersion        string
	NoPromisc        bool
	RoundtripFile    string
	RoundtripSize    int
	RoundtripBackups int
	Protocols        []string
}

type protoConfig struct {
	Name     string
	Protocol string
	Ports    []uint16
	Host     string
}

func (c *watchCmdConfig) decodeProtoConfig() ([]protoConfig, error) {
	var pcs []protoConfig
	for idx, proto := range c.Protocols {
		parts := strings.Split(proto, ";")
		if len(parts) < 2 {
			return nil, errors.Errorf("invalid --proto input '%s', expected 'protocol;ports[;host]' ('http;80,8080;127.0.0.1')", proto)
		}

		var pc protoConfig
		for _, port := range strings.Split(parts[1], ",") {
			i, err := strconv.ParseUint(port, 10, 16)
			if err != nil {
				return nil, errors.Errorf("invalid port '%s' in protocol '%s', port must between 0-65535", port, proto)
			}
			pc.Ports = append(pc.Ports, uint16(i))
		}

		pc.Name = strconv.Itoa(idx)
		pc.Protocol = parts[0]

		if len(parts) > 2 {
			pc.Host = parts[2]
		}
		pcs = append(pcs, pc)
	}
	return pcs, nil
}

func (c *watchCmdConfig) Yaml() ([]byte, error) {
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
  ipVersion: {{ .IPVersion }}
  noPromisc: {{ .NoPromisc }}
  protocols:
    rules:
{{ range .Protos }}
    - name: {{ .Name }}
      protocol: {{ .Protocol }}
      ports:
{{- range .Ports }}
        - {{ . }}	
{{- end }}
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
		return nil, err
	}

	ports, err := c.decodeProtoConfig()
	if err != nil {
		return nil, err
	}
	if len(ports) == 0 {
		return nil, errors.New("no valid protocols, please use --proto to add protocols")
	}

	var buf bytes.Buffer
	err = tpl.Execute(&buf, map[string]interface{}{
		"File":             c.File,
		"Console":          c.Console,
		"Ifaces":           c.Ifaces,
		"IPVersion":        c.IPVersion,
		"NoPromisc":        c.NoPromisc,
		"Protos":           ports,
		"RoundtripFile":    c.RoundtripFile,
		"RoundtripSize":    c.RoundtripSize,
		"RoundtripBackups": c.RoundtripBackups,
	})
	return buf.Bytes(), err
}

var watchConfig watchCmdConfig

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Capture and log network traffic roundtrips",
	Run: func(cmd *cobra.Command, args []string) {
		content, err := watchConfig.Yaml()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
			os.Exit(1)
		}

		cfg, err := confengine.LoadContent(content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
			os.Exit(1)
		}

		ctr, err := controller.New(cfg, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create controller: %v\n"+
				"Note: This operation may requires root privileges (try running with 'sudo')\n", err)
			os.Exit(1)
		}
		if err := ctr.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to start controller: %v\n", err)
			os.Exit(1)
		}

		<-sigs.Terminate()
		ctr.Stop()
	},
	Example: "# packetd watch --proto 'http;80,8080' --proto 'dns;53;127.0.0.1' --ifaces any --console",
}

func init() {
	watchCmd.Flags().BoolVar(&watchConfig.Console, "console", false, "Enable console logging")
	watchCmd.Flags().BoolVar(&watchConfig.NoPromisc, "no-promisc", false, "Don't put the interface into promiscuous mode")
	watchCmd.Flags().StringVar(&watchConfig.File, "pcap-file", "", "Path to pcap file to read from")
	watchCmd.Flags().StringVar(&watchConfig.Ifaces, "ifaces", "any", "Network interfaces to monitor (supports regex), 'any' for all interfaces")
	watchCmd.Flags().StringArrayVar(&watchConfig.Protocols, "proto", nil, "Protocols to capture in 'protocol;ports[;host]' format, multiple protocols supported")
	watchCmd.Flags().StringVar(&watchConfig.IPVersion, "ipv", "", "Filter by IP version [v4|v6]. Defaults to both")
	watchCmd.Flags().StringVar(&watchConfig.RoundtripFile, "roundtrips.file", "packetd.roundtrips", "Path to roundtrips file")
	watchCmd.Flags().IntVar(&watchConfig.RoundtripSize, "roundtrips.size", 100, "Maximum size of roundtrips file in MB")
	watchCmd.Flags().IntVar(&watchConfig.RoundtripBackups, "roundtrips.backups", 10, "Maximum number of old roundtrips files to retain")
	rootCmd.AddCommand(watchCmd)
}
