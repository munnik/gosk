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
	"github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var reverseMapCmd = &cobra.Command{
	Use:   "reverseMap",
	Short: "Create raw messages from mapped messages",
	Long:  `Create raw messages from signalK data, for sending back to a connector for example`,
	Run:   doReverseMap,
}

func init() {
	rootCmd.AddCommand(reverseMapCmd)
	reverseMapCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	reverseMapCmd.MarkFlagRequired("subscribeURL")
	reverseMapCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	reverseMapCmd.MarkFlagRequired("publishURL")
}

func doReverseMap(cmd *cobra.Command, args []string) {
	publisher := nanomsg.NewPublisher[message.Raw](publishURL)

	c := config.NewMapperConfig(cfgFile)
	switch c.Protocol {
	case config.ModbusType:
		subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		rmc := config.NewModbusMappingsConfig(cfgFile)
		m, err := mapper.NewModbusRawMapper(c, rmc)
		if err != nil {
			logger.GetLogger().Fatal(
				"Error while creating the mapper",
				zap.String("Config file", cfgFile),
				zap.String("Error", err.Error()),
			)
		}
		m.Map(subscriber, publisher)
	default:
		logger.GetLogger().Fatal(
			"Not a supported protocol",
			zap.String("Protocol", c.Protocol),
		)
		return
	}
}
