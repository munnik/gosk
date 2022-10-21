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
	"github.com/spf13/cobra"
)

var (
	replayCmd = &cobra.Command{
		Use:   "replay",
		Short: "Replay data from the database",
		Long:  `Replay raw or mapped data from the database`,
		Run:   doReplay,
	}
)

func init() {
	rootCmd.AddCommand(replayCmd)
	replayCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the data on. It listens for connections.")
	replayCmd.MarkFlagRequired("publishURL")
}

func doReplay(cmd *cobra.Command, args []string) {
	// publisher := nanomsg.NewPub(publishURL)
}
