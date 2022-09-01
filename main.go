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
package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/munnik/gosk/cmd"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

// var version = "undefined" // overwritten by Makefile

func main() {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel)
	go func() {
		for {
			s := <-signalChannel
			// https://www.computerhope.com/unix/signals.htm
			if s == syscall.SIGQUIT || s == syscall.SIGTERM || s == syscall.SIGINT {
				logger.GetLogger().Error(
					"Receive a signal from to OS to stop the application",
					zap.String("Signal", s.String()),
				)
				os.Exit(0)
			}
		}
	}()

	cmd.Execute()
}
