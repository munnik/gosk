package config

import (
	"time"

	"github.com/mcuadros/go-defaults"
	"github.com/mitchellh/mapstructure"
	"github.com/munnik/gosk/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func readConfigFile(result interface{}, configFilePath string, subKeys ...string) interface{} {
	defaults.SetDefaults(result)
	viper.SetConfigFile(configFilePath)
	if err := viper.ReadInConfig(); err != nil {
		logger.GetLogger().Fatal(
			"Fatal error while reading the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
		return result
	}

	if len(subKeys) > 1 {
		logger.GetLogger().Fatal(
			"Unable to read the configuration, only one key is allowed",
			zap.String("Config file", configFilePath),
			zap.Strings("Keys", subKeys),
		)
		return result
	}

	var err error
	if len(subKeys) == 0 {
		err = viper.Unmarshal(
			result,
			viper.DecodeHook(
				mapstructure.ComposeDecodeHookFunc(
					mapstructure.StringToTimeHookFunc(time.RFC3339),
					mapstructure.StringToTimeDurationHookFunc(),
				),
			),
		)
	} else {
		err = viper.UnmarshalKey(subKeys[0], result)
	}
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}
	return result
}
