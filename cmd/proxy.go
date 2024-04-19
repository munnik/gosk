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
	"sync"

	"github.com/munnik/gosk/nanomsg"
	"github.com/spf13/cobra"
)

var (
	proxyCmd = &cobra.Command{
		Use:   "proxy",
		Short: "Proxy for Nanomsg",
		Long:  `This proxy can connect to multiple publishers and serve multiple subscribers`,
		Run:   doProxy,
	}
	proxySubscribeURLs []string
)

func init() {
	rootCmd.AddCommand(proxyCmd)
	proxyCmd.Flags().StringVarP(&publishURL, "publishURL", "p", "", "Nanomsg URL, the URL is used to publish the collected data on. It listens for connections.")
	proxyCmd.MarkFlagRequired("publishURL")
	proxyCmd.Flags().StringSliceVarP(&proxySubscribeURLs, "subscribeURL", "s", []string{}, "Nanomsg URL, the URL is used to listen for subscribed data.")
}

func doProxy(cmd *cobra.Command, args []string) {
	proxy := nanomsg.NewProxy(publishURL)
	defer proxy.Close()
	var wg sync.WaitGroup
	wg.Add(len(proxySubscribeURLs))
	for _, proxySubscribeURL := range proxySubscribeURLs {
		go proxy.SubscribeTo(proxySubscribeURL, &wg)
	}
	wg.Wait()
}
