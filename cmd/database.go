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
	log "github.com/sirupsen/logrus"

	"github.com/munnik/gosk/database/keyvalue"
	"github.com/munnik/gosk/database/raw"
	"github.com/munnik/gosk/nanomsg"
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
	keyValueDatabaseCmd = &cobra.Command{
		Use:   "keyvalue",
		Short: "Store key-value messages in the timeseries database",
		Long:  `Store key-value messages in the timeseries database`,
		Run:   keyValueDatabase,
	}
	databaseSubscribeURI string
)

func init() {
	rootCmd.AddCommand(databaseCmd)
	databaseCmd.AddCommand(rawDatabaseCmd)
	databaseCmd.AddCommand(keyValueDatabaseCmd)
	rawDatabaseCmd.Flags().StringVarP(&databaseSubscribeURI, "subscribeURI", "s", "", "Nanomsg URI, the URI is used to listen for subscribed data.")
	rawDatabaseCmd.MarkFlagRequired("subscribeURI")
	keyValueDatabaseCmd.Flags().StringVarP(&databaseSubscribeURI, "subscribeURI", "s", "", "Nanomsg URI, the URI is used to listen for subscribed data.")
	keyValueDatabaseCmd.MarkFlagRequired("subscribeURI")
}

func rawDatabase(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(databaseSubscribeURI, []byte{})
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
	raw.Store(subscriber)
	for {
	}
}

func keyValueDatabase(cmd *cobra.Command, args []string) {
	subscriber, err := nanomsg.NewSub(databaseSubscribeURI, []byte{})
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
	keyvalue.Store(subscriber)
	for {
	}
}
