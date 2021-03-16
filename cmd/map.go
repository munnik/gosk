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
	"os"

	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk/mapper"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	mapCmd = &cobra.Command{
		Use:   "map",
		Short: "Map raw data to meaningfull data",
		Long:  `Map raw data to meaningfull data using the SignalK`,
		Run:   doMap,
	}
	mapSubscribeURI string
	mapPublishURI   string
)

func init() {
	rootCmd.AddCommand(mapCmd)
	mapCmd.Flags().StringVarP(&mapPublishURI, "publishURI", "u", "", "Nanomsg URI, the URI is used to publish the collected data on. It listens for connections.")
	mapCmd.MarkFlagRequired("publishURI")
	mapCmd.Flags().StringVarP(&mapSubscribeURI, "subscribeURI", "s", "", "Nanomsg URI, the URI is used to listen for subscribed data.")
	mapCmd.MarkFlagRequired("subscribeURI")
}

func doMap(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(mapSubscribeURI, []byte{})
	logger.Fatal(
		"Could not parse the URI",
		zap.String("URI", mapSubscribeURI),
		zap.Error(err),
	)
	os.Exit(1)
	mapper.Map(subscriber, nanomsg.NewPub(mapPublishURI))
	for {
	}
}
