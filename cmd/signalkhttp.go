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
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/writer"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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
	signalKHTTPCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	signalKHTTPCmd.MarkFlagRequired("subscribeURL")
}

func serveSignalKHTTP(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewSignalKConfig(cfgFile).WithVersion(version)
	a := writer.NewHTTPWriter(c)
	a.WriteMapped(subscriber)
}
