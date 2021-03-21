package modbus

import (
	"github.com/munnik/gosk/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type RegisterMapping struct {
	Size        uint16   `mapstructure:"Size"`
	Function    string   `mapstructure:"Function"`
	SignalKPath []string `mapstructure:"SignalKPath"`
}

type MappingConfig struct {
	RegisterMappings map[uint16]RegisterMapping `mapstructure:"Mappings"`
	Context          string                     `mapstructure:"Context"`
}

func CreateConfig(configFilePath string) MappingConfig {
	var result MappingConfig

	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.Unmarshal(&result)
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}

	return result
}
