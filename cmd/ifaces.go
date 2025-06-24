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
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"
)

var ifacesCmd = &cobra.Command{
	Use:   "ifaces",
	Short: "List all available interfaces",
	Run: func(cmd *cobra.Command, args []string) {
		ifaces, err := net.Interfaces()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list interfaces: %v\n", err)
			os.Exit(1)
		}

		for _, iface := range ifaces {
			addr, err := iface.Addrs()
			if err != nil {
				continue
			}
			fmt.Printf("- %s: %v\n", iface.Name, addr)
		}
	},
}

func init() {
	rootCmd.AddCommand(ifacesCmd)
}
