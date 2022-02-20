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
	"fmt"
	"path"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/spf13/viper"
)

var (
	cfgFile      string
	subscribeURL string
	publishURL   string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gosk",
	Short: "Gosk collects, processes, stores and publishes information from one or more vessels",
	Long: `Gosk collects data using one or more collectors. There are specific collectors 
for different protocols (e.g. NMEA0183, NMEA200, Canbus or Modbus). This raw data 
can be stored in a database for later usage. The data can also be forwarded to a 
mapper to process the data and convert it to SignalK key/value pairs. Finally the 
data can be published in different ways (e.g. HTTP or Websocket).`,
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
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to config file")
}

// initConfig reads in config file and ENV variables if set.
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

func getProtocol(cfgFilePath string) (string, error) {
	strippedPath := cfgFilePath
	supportedProtocols := map[string]struct{}{
		config.NMEA0183Type: {},
		config.ModbusType:   {},
		config.CsvType:      {},
	}
	for len(strippedPath) > 0 && strippedPath != "." {
		if _, ok := supportedProtocols[path.Base(strippedPath)]; ok {
			return path.Base(strippedPath), nil
		}
		strippedPath = path.Dir(strippedPath)
	}

	return "", fmt.Errorf("the path %s does not contain a supported protocol", cfgFilePath)
}
