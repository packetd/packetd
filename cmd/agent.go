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
	"os"

	"github.com/spf13/cobra"

	"github.com/packetd/packetd/confengine"
	"github.com/packetd/packetd/controller"
	"github.com/packetd/packetd/internal/sigs"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run packetd as a network monitoring agent",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := confengine.LoadConfigPath(configPath)
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
}

var configPath string

func init() {
	agentCmd.Flags().StringVar(&configPath, "config", "packetd.yaml", "Configuration file path")
	rootCmd.AddCommand(agentCmd)
}
