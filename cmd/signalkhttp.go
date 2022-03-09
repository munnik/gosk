/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"github.com/munnik/gosk/api"
	"github.com/munnik/gosk/config"
	"github.com/spf13/cobra"
)

var (
	signalKHTTPCmd = &cobra.Command{
		Use:   "http",
		Short: "SignalK HTTP",
		Long:  `Starts a HTTP server that publishes SignalK full models`,
		Run:   serveSignalKHTTP,
	}
)

func init() {
	rootCmd.AddCommand(signalKHTTPCmd)
}

func serveSignalKHTTP(cmd *cobra.Command, args []string) {
	c := config.NewSignalKConfig(cfgFile).WithVersion(version)
	a := api.NewSignalKAPI(c)
	a.ServeSignalK()
}
