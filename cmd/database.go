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

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/writer"
	"github.com/spf13/cobra"
)

var (
	databaseCmd = &cobra.Command{
		Use:   "database",
		Short: "Store messages in the timeseries database",
		Long:  `Store messages in the timeseries database`,
	}
	rawDatabaseCmd = &cobra.Command{
		Use:   "raw",
		Short: "Store raw messages in the timeseries database",
		Long:  `Store raw messages in the timeseries database`,
		Run:   rawDatabase,
	}
	mappedDatabaseCmd = &cobra.Command{
		Use:   "mapped",
		Short: "Store mapped messages in the timeseries database",
		Long:  `Store mapped messages in the timeseries database`,
		Run:   mappedDatabase,
	}
	databaseSubscribeURI string
	databaseURI          string
)

func init() {
	rootCmd.AddCommand(databaseCmd)
	databaseCmd.AddCommand(rawDatabaseCmd)
	databaseCmd.AddCommand(mappedDatabaseCmd)
	rawDatabaseCmd.Flags().StringVarP(&databaseSubscribeURI, "subscribeURI", "s", "", "Nanomsg URI, the URI is used to listen for subscribed data.")
	rawDatabaseCmd.MarkFlagRequired("subscribeURI")
	rawDatabaseCmd.Flags().StringVarP(&databaseURI, "databaseURI", "d", "", "The URI used to connect to the database")
	rawDatabaseCmd.MarkFlagRequired("databaseURI")
	mappedDatabaseCmd.Flags().StringVarP(&databaseSubscribeURI, "subscribeURI", "s", "", "Nanomsg URI, the URI is used to listen for subscribed data.")
	mappedDatabaseCmd.MarkFlagRequired("subscribeURI")
	mappedDatabaseCmd.Flags().StringVarP(&databaseURI, "databaseURI", "d", "", "The URI used to connect to the database")
	mappedDatabaseCmd.MarkFlagRequired("databaseURI")
}

func rawDatabase(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(databaseSubscribeURI, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URI",
			zap.String("URI", databaseSubscribeURI),
			zap.String("Error", err.Error()),
		)
	}
	w := writer.NewPostgresqlWriter().WithUrl(databaseURI)
	w.WriteRaw(subscriber)
}

func mappedDatabase(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(databaseSubscribeURI, []byte{})
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the URI",
			zap.String("URI", databaseSubscribeURI),
			zap.String("Error", err.Error()),
		)
	}
	w := writer.NewPostgresqlWriter().WithUrl(databaseURI)
	w.WriteMapped(subscriber)
}
