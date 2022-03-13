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
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/reader"
	"github.com/spf13/cobra"
)

var (
	readCmd = &cobra.Command{
		Use:   "read",
		Short: "Transmit data via a mqtt broker",
		Long:  `Transmit data via a mqtt broker`,
	}
	mqttReadCmd = &cobra.Command{
		Use:   "mqtt",
		Short: "Read messages from a broker",
		Long:  `Read messages from a broker`,
		Run:   doMQTTRead,
	}
)

func init() {
	rootCmd.AddCommand(readCmd)

	readCmd.AddCommand(mqttReadCmd)
	mqttReadCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	mqttReadCmd.MarkFlagRequired("publishURL")
}

func doMQTTRead(cmd *cobra.Command, args []string) {
	c := config.NewMQTTConfig(cfgFile)
	r := reader.NewMqttReader(c)
	r.ReadMapped(nanomsg.NewPub(publishURL))
}
