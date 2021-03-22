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
	"net/url"

	"github.com/munnik/gosk/collector"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	collectCmd = &cobra.Command{
		Use:   "collect",
		Short: "Collect data using a specific protocol",
		Long:  `Collect data using a specific protocol`,
		Run:   collect,
	}
	collectPublishURI string
)

func init() {
	rootCmd.AddCommand(collectCmd)
	collectCmd.Flags().StringVarP(&collectPublishURI, "publishURI", "u", "", "Nanomsg URI, the URI is used to publish the collected data on. It listens for connections.")
	collectCmd.MarkFlagRequired("publishURI")
}

func collect(cmd *cobra.Command, args []string) {
	protocol, err := getProtocol(cfgFile)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not determine the protocol, make sure the protocol is in the path of the config file",
			zap.String("Config file", cfgFile),
			zap.String("Error", err.Error()),
		)
	}
	switch protocol {
	case config.NMEA0183Type:
		cfg := config.NewNMEA0183Config(cfgFile)
		uri, err := url.Parse(cfg.URI)
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not parse the URI",
				zap.String("URI", cfg.URI),
				zap.String("Error", err.Error()),
			)
		}
		if uri.Scheme == "tcp" || uri.Scheme == "udp" {
			collector.NewNMEA0183NetworkCollector(uri, cfg).Collect(nanomsg.NewPub(collectPublishURI))
		}
		if uri.Scheme == "file" {
			collector.NewNMEA0183FileCollector(uri, cfg).Collect(nanomsg.NewPub(collectPublishURI))
		}
	case config.ModbusType:
		cfg := config.NewModbusConfig(cfgFile)
		uri, err := url.Parse(cfg.URI)
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not parse the URI",
				zap.String("URI", cfg.URI),
				zap.String("Error", err.Error()),
			)
		}
		if uri.Scheme == "tcp" {
			modbusConfig := config.NewModbusConfig(cfgFile)
			collector.NewModbusNetworkCollector(uri, modbusConfig).Collect(nanomsg.NewPub(collectPublishURI))
		}
	default:
		logger.GetLogger().Fatal(
			"Not a supported protocol",
			zap.String("Protocol", protocol),
		)
	}
}
