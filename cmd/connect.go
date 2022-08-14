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

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/connector"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	connectCmd = &cobra.Command{
		Use:   "connect",
		Short: "Connect to a data source to receive (and optional send) data using a specific protocol",
		Long:  fmt.Sprintf(`Collect data using a specific protocol, current supported protocols are %v, %v, %v and %v`, config.NMEA0183Type, config.ModbusType, config.CSVType, config.JSONType),
		Run:   doConnect,
	}
)

func init() {
	rootCmd.AddCommand(connectCmd)
	connectCmd.Flags().StringVarP(&subscribeURL, "subscribeURL", "s", "", "Nanomsg URL, the URL is used to listen for subscribed data.")
	connectCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
}

func doConnect(cmd *cobra.Command, args []string) {
	var err error
	c := config.NewConnectorConfig(cfgFile)
	var conn connector.Connector
	switch c.Protocol {
	case config.CSVType, config.NMEA0183Type, config.JSONType:
		conn, err = connector.NewLineConnector(c)
	case config.ModbusType:
		rgc := config.NewRegisterGroupsConfig(cfgFile)
		conn, err = connector.NewModbusConnector(c, rgc)
	case config.CanBusType:
		conn, err = connector.NewCanBusConnector(c)
	default:
		logger.GetLogger().Fatal(
			"Not a supported protocol",
			zap.String("Protocol", c.Protocol),
		)
		return
	}
	if err != nil {
		logger.GetLogger().Fatal(
			"Error while creating the connector",
			zap.String("Config file", cfgFile),
			zap.String("Error", err.Error()),
		)
	}

	publisher := nanomsg.NewPub(publishURL)
	subscriber, err := nanomsg.NewSub(subscribeURL, []byte{})
	if err != nil && subscribeURL != "" {
		logger.GetLogger().Warn(
			"Error while creating the subscriber",
			zap.String("Subscribe URL", subscribeURL),
			zap.String("Error", err.Error()),
		)
	}
	conn.Connect(publisher, subscriber)
}
