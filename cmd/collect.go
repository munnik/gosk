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
	"fmt"

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
		Long:  fmt.Sprintf(`Collect data using a specific protocol, current supported protocols are %v, %v and %v`, config.NMEA0183Type, config.ModbusType, config.SygoType),
		Run:   doCollect,
	}
)

func init() {
	rootCmd.AddCommand(collectCmd)
	collectCmd.Flags().StringVarP(&publishURL, "publishURL", "u", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	collectCmd.MarkFlagRequired("publishURL")
}

func doCollect(cmd *cobra.Command, args []string) {
	protocol, err := getProtocol(cfgFile)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not determine the protocol, make sure the protocol is in the path of the config file",
			zap.String("Config file", cfgFile),
			zap.String("Error", err.Error()),
		)
	}
	c := config.NewCollectorConfig(cfgFile).WithProtocol(protocol)
	var reader collector.Collector
	switch protocol {
	case config.NMEA0183Type:
		reader, err = collector.NewLineReader(c)
	case config.SygoType:
		reader, err = collector.NewLineReader(c)
	case config.ModbusType:
		rgc := config.NewRegisterGroupsConfig(cfgFile)
		reader, err = collector.NewModbusReader(c, rgc)
	default:
		logger.GetLogger().Fatal(
			"Not a supported protocol",
			zap.String("Protocol", protocol),
		)
		return
	}
	if err != nil {
		logger.GetLogger().Fatal(
			"Error while creating the collector",
			zap.String("Config file", cfgFile),
			zap.String("Error", err.Error()),
		)
	}
	reader.Collect(nanomsg.NewPub(publishURL))
}
