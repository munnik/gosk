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
	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
)

var (
	proxyCmd = &cobra.Command{
		Use:   "proxy",
		Short: "Proxy for Nanomsg",
		Long:  `This proxy can connect to multiple publishers and serve multiple subscribers`,
		Run:   proxy,
	}
	proxyPublishURI   string
	proxySubscribeURI []string
)

func init() {
	rootCmd.AddCommand(proxyCmd)
	proxyCmd.Flags().StringVarP(&proxyPublishURI, "publishURI", "u", "", "Nanomsg URI, the URI is used to publish the collected data on. It listens for connections.")
	proxyCmd.MarkFlagRequired("publishURI")
	proxyCmd.Flags().StringSliceVarP(&proxySubscribeURI, "subscribeURI", "s", []string{}, "Nanomsg URI, the URI is used to listen for subscribed data.")

	nanomsg.Logger = Logger
}

func proxy(cmd *cobra.Command, args []string) {
	proxy := nanomsg.NewProxy(proxyPublishURI)
	defer proxy.Close()
	for _, publisher := range proxySubscribeURI {
		proxy.SubscribeTo(publisher)
	}
	for {
		// never exit
	}
}
