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
	protocol          string
	connectionURI     string
	connectionName    string
	dial              bool
	baudRate          int
	collectPublishURI string
)

func init() {
	rootCmd.AddCommand(collectCmd)
	collectCmd.Flags().StringVarP(&protocol, "protocol", "p", "", "Protocol to use for collection of data (required)")
	collectCmd.MarkFlagRequired("protocol")
	collectCmd.Flags().StringVarP(&connectionURI, "connectionURI", "c", "", "Connection URI, if URI refers to this machine then gosk listens for incoming connections, otherwise gosk will try to dial the remote system.")
	collectCmd.MarkFlagRequired("connectionURI")
	collectCmd.Flags().StringVarP(&connectionName, "connectionName", "n", "", "Name of the connection, this is used when logging/storing the data.")
	collectCmd.MarkFlagRequired("connectionName")
	collectCmd.Flags().BoolVarP(&dial, "dial", "d", false, "Forces to dial to connectionURI instead of listening, default behavior is to listen for network connections. This flag is ignored for file based connections.")
	collectCmd.Flags().IntVarP(&baudRate, "baudRate", "b", 4800, "Baud rate for serial connections, default is 4800 baud.")
	collectCmd.Flags().StringVarP(&collectPublishURI, "publishURI", "u", "", "Nanomsg URI, the URI is used to publish the collected data on. It listens for connections.")
	collectCmd.MarkFlagRequired("publishURI")

	collector.Logger = Logger
}

func collect(cmd *cobra.Command, args []string) {
	switch protocol {
	case "nmea0183":
		uri, err := url.Parse(connectionURI)
		if err != nil {
			Logger.Fatal(
				"Could not parse the URI",
				zap.String("URI", connectionURI),
				zap.String("Error", err.Error()),
			)
		}
		if uri.Scheme == "tcp" || uri.Scheme == "udp" {
			collector.NewNMEA0183NetworkCollector(uri, dial, connectionName).Collect(nanomsg.NewPub(collectPublishURI))
		}
		if uri.Scheme == "file" {
			collector.NewNMEA0183FileCollector(uri, baudRate, connectionName).Collect(nanomsg.NewPub(collectPublishURI))
		}
	default:
		Logger.Fatal(
			"Not a supported protocol",
			zap.String("Protocol", protocol),
		)
	}
}
