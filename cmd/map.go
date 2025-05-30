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

var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Map raw data to meaningful data",
	Long:  `Map raw data to meaningful data based on the SignalK specification`,
	Run:   doMap,
}

func init() {
	rootCmd.AddCommand(mapCmd)
	mapCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	mapCmd.MarkFlagRequired("subscribeURL")
	mapCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	mapCmd.MarkFlagRequired("publishURL")
}

func doMap(cmd *cobra.Command, args []string) {
	publisher := nanomsg.NewPublisher[message.Mapped](publishURL)

	c := config.NewMapperConfig(cfgFile)
	switch c.Protocol {
	case config.CSVType:
		subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		c2 := config.NewCSVMapperConfig(cfgFile)
		cmc := config.NewCSVMappingConfig(cfgFile)
		m, err := mapper.NewCSVMapper(c2, cmc)
		if err != nil {
			logger.GetLogger().Fatal(
				"Error while creating the mapper",
				zap.String("Config file", cfgFile),
				zap.String("Error", err.Error()),
			)
		}
		m.Map(subscriber, publisher)
	case config.JSONType:
		subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		jmc := config.NewJSONMappingConfig(cfgFile)
		m, err := mapper.NewJSONMapper(c, jmc)
		if err != nil {
			logger.GetLogger().Fatal(
				"Error while creating the mapper",
				zap.String("Config file", cfgFile),
				zap.String("Error", err.Error()),
			)
		}
		m.Map(subscriber, publisher)
	case config.ModbusType:
		subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		rmc := config.NewModbusMappingsConfig(cfgFile)
		m, err := mapper.NewModbusMapper(c, rmc)
		if err != nil {
			logger.GetLogger().Fatal(
				"Error while creating the mapper",
				zap.String("Config file", cfgFile),
				zap.String("Error", err.Error()),
			)
		}
		m.Map(subscriber, publisher)
	case config.NMEA0183Type:
		subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		m, err := mapper.NewNmea0183Mapper(c)
		if err != nil {
			logger.GetLogger().Fatal(
				"Error while creating the mapper",
				zap.String("Config file", cfgFile),
				zap.String("Error", err.Error()),
			)
		}
		m.Map(subscriber, publisher)
	case config.CanBusType:
		subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		c2 := config.NewCanBusMapperConfig(cfgFile)
		cmc := config.NewCanBusMappingConfig(cfgFile)
		m, err := mapper.NewCanBusMapper(c2, cmc)
		if err != nil {
			logger.GetLogger().Fatal(
				"Error while creating the mapper",
				zap.String("Config file", cfgFile),
				zap.String("Error", err.Error()),
			)
		}
		m.Map(subscriber, publisher)
	case config.SignalKType:
		subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		amc := config.NewExpressionMappingConfig(cfgFile)
		m, err := mapper.NewAggregateMapper(c, amc)
		if err != nil {
			logger.GetLogger().Fatal(
				"Error while creating the mapper",
				zap.String("Config file", cfgFile),
				zap.String("Error", err.Error()),
			)
		}
		m.Map(subscriber, publisher)
	case config.FftType:
		subscriber, err := nanomsg.NewSubscriber[message.Mapped](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		fftc := config.NewFftConfig(cfgFile)
		m, err := mapper.NewFftMapper(c, fftc)
		if err != nil {
			logger.GetLogger().Fatal(
				"Error while creating the mapper",
				zap.String("Config file", cfgFile),
				zap.String("Error", err.Error()),
			)
		}
		m.Map(subscriber, publisher)
	case config.BinaryType:
		subscriber, err := nanomsg.NewSubscriber[message.Raw](subscribeURL, []byte{})
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not subscribe",
				zap.String("URL", subscribeURL),
				zap.String("Error", err.Error()),
			)
		}
		mc := config.NewMappingConfig(cfgFile)
		m, err := mapper.NewBinaryMapper(c, mc)
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
