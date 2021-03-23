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
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalkws"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	signalKWsCmd = &cobra.Command{
		Use:   "ws",
		Short: "SignalK websocket",
		Long:  `Starts a websocket that publishes SignalK deltas`,
		Run:   serveSignalKWs,
	}
	signalKWsSubscribeURI string
)

func init() {
	rootCmd.AddCommand(signalKWsCmd)
	signalKWsCmd.Flags().StringVarP(&signalKWsSubscribeURI, "subscribeURI", "s", "", "Nanomsg URI, the URI is used to listen for subscribed data.")
	signalKWsCmd.MarkFlagRequired("subscribeURI")
}

func serveSignalKWs(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(signalKWsSubscribeURI, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URI",
			zap.String("URI", signalKWsSubscribeURI),
			zap.String("Error", err.Error()),
		)
	}

	signalkws.Start(subscriber)
}
