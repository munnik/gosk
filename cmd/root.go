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
	"log"
	"net/http"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/spf13/viper"
)

var (
	cfgFile                 string
	profilingAndMetricsPort string
	subscribeURL            string
	publishURL              string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gosk",
	Short: "Gosk collects, processes, stores and publishes information from one or more vessels",
	Long: `Gosk collects data using one or more connectors. There are specific connectors 
for different protocols (e.g. NMEA0183, NMEA200, Canbus or Modbus). This raw data 
can be stored in a database for later usage. The data can also be forwarded to a 
mapper to process the data and convert it to SignalK key/value pairs. Finally the 
data can be published in different ways (e.g. HTTP or Websocket).`,
	Version: version.Version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.GetLogger().Fatal(
			"Could not execute the Cobra root command",
			zap.String("Error", err.Error()),
		)
	}
}

func init() {
	cobra.OnInitialize(
		initConfig,
		initProfilingAndMetrics,
	)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file")
	rootCmd.PersistentFlags().StringVar(&profilingAndMetricsPort, "pmport", "", "port to run the http server for pprof and prometheus")
}

// initConfig reads in config file
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
		// If a config file is found, read it in.
		if err := viper.ReadInConfig(); err == nil {
			logger.GetLogger().Info(
				"Config file used",
				zap.String("File", viper.ConfigFileUsed()),
			)
		}
	}
}

func initProfilingAndMetrics() {
	if profilingAndMetricsPort != "" {
		http.Handle("/metrics", promhttp.Handler())
		go func() {
			err := http.ListenAndServe(profilingAndMetricsPort, nil)
			if err != nil {
				log.Fatal(
					"Could not start profiling and metrics",
					zap.String("Host and port", profilingAndMetricsPort),
					zap.String("Error", err.Error()),
				)
			}
		}()
		logger.GetLogger().Info(
			"Starting profiling and metrics",
			zap.String("Host and port", profilingAndMetricsPort),
		)
	}
}
