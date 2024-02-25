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
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/version"
	"github.com/munnik/gosk/writer"
	"github.com/spf13/cobra"
)

var (
	writeCmd = &cobra.Command{
		Use:   "write",
		Short: "Write messages to a storage or remote location",
		Long:  `Write messages to a storage or remote location`,
	}
	writeDatabaseCmd = &cobra.Command{
		Use:   "database",
		Short: "Store messages in the timeseries database",
		Long:  `Store messages in the timeseries database`,
	}
	writeDatabaseRawCmd = &cobra.Command{
		Use:   "raw",
		Short: "Store raw messages in the timeseries database",
		Long:  `Store raw messages in the timeseries database`,
		Run:   doWriteDatabaseRaw,
	}
	writeDatabaseMappedCmd = &cobra.Command{
		Use:   "mapped",
		Short: "Store mapped messages in the timeseries database",
		Long:  `Store mapped messages in the timeseries database`,
		Run:   doWriteDatabaseMapped,
	}
	writeMQTTCmd = &cobra.Command{
		Use:   "mqtt",
		Short: "Write messages to a broker",
		Long:  `Write messages to a broker`,
		Run:   doWriteMQTT,
	}
	writeSignalKCmd = &cobra.Command{
		Use:   "signalk",
		Short: "SignalK HTTP",
		Long:  `Starts a HTTP server that publishes SignalK full models`,
		Run:   doWriteSignalK,
	}
	writeStdOutCmd = &cobra.Command{
		Use:   "stdout",
		Short: "Write messages to stdout",
		Long:  `Write messages to stdout`,
	}
	writeStdOutRawCmd = &cobra.Command{
		Use:   "raw",
		Short: "write raw messages to stdout",
		Long:  `write raw messages to stdout`,
		Run:   doWriteStdOutRaw,
	}
	writeStdOutRawStringCmd = &cobra.Command{
		Use:   "rawstring",
		Short: "write raw messages to stdout",
		Long:  `write raw messages to stdout, the bytes are converted to a string`,
		Run:   doWriteStdOutRawString,
	}
	writeStdOutMappedCmd = &cobra.Command{
		Use:   "mapped",
		Short: "write mapped messages to stdout",
		Long:  `write mapped messages to stdout`,
		Run:   doWriteStdOutMapped,
	}
	writeLWECmd = &cobra.Command{
		Use:   "lwe",
		Short: "Write messages to an UDP multicast group",
		Long:  `Write messages to an UDP multicast group according to the LWE (IEC 61162-450) protocol`,
		Run:   doWriteLWE,
	}
	writeGrafanaCmd = &cobra.Command{
		Use:   "grafana",
		Short: "Write messages to an MQTT broker for grafana",
		Long:  `Write messages to an MQTT broker for grafana`,
		Run:   doWriteGrafana,
	}
)

func init() {
	rootCmd.AddCommand(writeCmd)

	writeCmd.AddCommand(writeDatabaseCmd)
	writeDatabaseCmd.AddCommand(writeDatabaseRawCmd)
	writeDatabaseRawCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeDatabaseRawCmd.MarkFlagRequired("subscribeURL")
	writeDatabaseCmd.AddCommand(writeDatabaseMappedCmd)
	writeDatabaseMappedCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeDatabaseMappedCmd.MarkFlagRequired("subscribeURL")

	writeCmd.AddCommand(writeMQTTCmd)
	writeMQTTCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeMQTTCmd.MarkFlagRequired("subscribeURL")

	writeCmd.AddCommand(writeSignalKCmd)
	writeSignalKCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeSignalKCmd.MarkFlagRequired("subscribeURL")

	writeCmd.AddCommand(writeStdOutCmd)
	writeStdOutCmd.AddCommand(writeStdOutRawCmd)
	writeStdOutRawCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeStdOutRawCmd.MarkFlagRequired("subscribeURL")
	writeStdOutCmd.AddCommand(writeStdOutRawStringCmd)
	writeStdOutRawStringCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeStdOutRawStringCmd.MarkFlagRequired("subscribeURL")
	writeStdOutCmd.AddCommand(writeStdOutMappedCmd)
	writeStdOutMappedCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeStdOutMappedCmd.MarkFlagRequired("subscribeURL")

	writeCmd.AddCommand(writeLWECmd)
	writeLWECmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeLWECmd.MarkFlagRequired("subscribeURL")

	writeCmd.AddCommand(writeGrafanaCmd)
	writeGrafanaCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	writeGrafanaCmd.MarkFlagRequired("subscribeURL")
}

func doWriteDatabaseRaw(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewPostgresqlConfig(cfgFile)
	w := writer.NewPostgresqlWriter(c)
	go w.StartRawWorkers()
	w.WriteRaw(subscriber)
}

func doWriteDatabaseMapped(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewPostgresqlConfig(cfgFile)
	w := writer.NewPostgresqlWriter(c)
	go w.StartMappedWorkers()
	w.WriteMapped(subscriber)
}

func doWriteMQTT(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
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

func doWriteSignalK(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewSignalKConfig(cfgFile).WithVersion(version.Version)
	s := writer.NewSignalKWriter(c)
	s.WriteMapped(subscriber)
}

func doWriteLWE(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewLWEConfig(cfgFile)
	w := writer.NewLWEWriter(c)
	w.WriteRaw(subscriber)
}

func doWriteStdOutMapped(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	s := writer.NewStdOutWriter()
	s.WriteMapped(subscriber)
}

func doWriteStdOutRaw(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	s := writer.NewStdOutWriter()
	s.WriteRaw(subscriber)
}

func doWriteStdOutRawString(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	s := writer.NewStdOutWriter()
	s.WriteRawString(subscriber)
}

func doWriteGrafana(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewMQTTConfig(cfgFile)
	w := writer.NewGrafanaWriter(c)
	w.WriteMapped(subscriber)
}
