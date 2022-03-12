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
	"go.uber.org/zap"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/writer"
	"github.com/spf13/cobra"
)

var (
	writeCmd = &cobra.Command{
		Use:   "write",
		Short: "Write messages to a storage or remote location",
		Long:  `Write messages to a storage or remote location`,
	}
	databaseCmd = &cobra.Command{
		Use:   "database",
		Short: "Store messages in the timeseries database",
		Long:  `Store messages in the timeseries database`,
	}
	rawDatabaseCmd = &cobra.Command{
		Use:   "raw",
		Short: "Store raw messages in the timeseries database",
		Long:  `Store raw messages in the timeseries database`,
		Run:   rawDatabase,
	}
	mappedDatabaseCmd = &cobra.Command{
		Use:   "mapped",
		Short: "Store mapped messages in the timeseries database",
		Long:  `Store mapped messages in the timeseries database`,
		Run:   mappedDatabase,
	}
	mqttWriteCmd = &cobra.Command{
		Use:   "mqtt",
		Short: "Write messages to a broker",
		Long:  `Write messages to a broker`,
		Run:   writeMQTT,
	}
	signalKHTTPCmd = &cobra.Command{
		Use:   "http",
		Short: "SignalK HTTP",
		Long:  `Starts a HTTP server that publishes SignalK full models`,
		Run:   serveSignalKHTTP,
	}
)

func init() {
	rootCmd.AddCommand(writeCmd)

	writeCmd.AddCommand(databaseCmd)
	databaseCmd.AddCommand(rawDatabaseCmd)
	rawDatabaseCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	rawDatabaseCmd.MarkFlagRequired("subscribeURL")
	databaseCmd.AddCommand(mappedDatabaseCmd)
	mappedDatabaseCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	mappedDatabaseCmd.MarkFlagRequired("subscribeURL")

	writeCmd.AddCommand(mqttWriteCmd)
	mqttWriteCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	mqttWriteCmd.MarkFlagRequired("subscribeURL")

	writeCmd.AddCommand(signalKHTTPCmd)
	signalKHTTPCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	signalKHTTPCmd.MarkFlagRequired("subscribeURL")
}

func rawDatabase(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewPostgresqlConfig(cfgFile)
	w := writer.NewPostgresqlWriter(c)
	w.WriteRaw(subscriber)
}

func mappedDatabase(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewPostgresqlConfig(cfgFile)
	w := writer.NewPostgresqlWriter(c)
	w.WriteMapped(subscriber)
}

func writeMQTT(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewMQTTConfig(cfgFile)
	w := writer.NewMqttWriter(c)
	w.WriteMapped(subscriber)
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
