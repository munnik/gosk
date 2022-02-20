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
	"github.com/munnik/gosk/reader"
	"github.com/munnik/gosk/writer"
	"github.com/spf13/cobra"
)

var (
	mqttCmd = &cobra.Command{
		Use:   "mqtt",
		Short: "Transmit data via a mqtt broker",
		Long:  `Transmit data via a mqtt broker`,
	}
	mqttWriteCmd = &cobra.Command{
		Use:   "write",
		Short: "Write messages to a broker",
		Long:  `Write messages to a broker`,
		Run:   writeMqtt,
	}
	mqttReadCmd = &cobra.Command{
		Use:   "read",
		Short: "Read messages from a broker",
		Long:  `Read messages from a broker`,
		Run:   readMqtt,
	}
	mqttBrokerURL string
)

func init() {
	rootCmd.AddCommand(mqttCmd)
	mqttCmd.AddCommand(mqttWriteCmd)
	mqttCmd.AddCommand(mqttReadCmd)
	mqttWriteCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	mqttWriteCmd.MarkFlagRequired("subscribeURL")
	mqttReadCmd.Flags().StringVarP(&mqttBrokerURL, "mqttBroker", "u", "", "MQTT broker URL, the URL is used to send the data to.")
	mqttReadCmd.MarkFlagRequired("mqttBroker")
}

func writeMqtt(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URL",
			zap.String("URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewMqttConfig(cfgFile)
	w := writer.NewMqttWriter(c)
	w.WriteMapped(subscriber)
}

func readMqtt(cmd *cobra.Command, args []string) {
	c := config.NewMqttConfig(cfgFile)
	r := reader.NewMqttReader(c)
	r.ReadMapped(nanomsg.NewPub(mqttBrokerURL))
}
