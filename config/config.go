package config

import (
	"time"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	// NMEA0183Type is used to identify the data as NMEA 0183 data
	NMEA0183Type = "NMEA0183"
	// ModbusType is used to identify the data as Modbus data
	ModbusType = "MODBUS"
)

type ModbusRegister struct {
	FunctionCode       uint16   `mapstructure:"FunctionCode"`
	Size               uint16   `mapstructure:"Size"`
	SignalKPath        []string `mapstructure:"SignalKPath"`
	Expression         string   `mapstructure:"Expression"`
	CompiledExpression *vm.Program
}

type ModbusRegisterMap map[uint16]*ModbusRegister

type ModbusConfig struct {
	Name             string            `mapstructure:"Name"`
	Context          string            `mapstructure:"Context"`
	URI              string            `mapstructure:"URI"`
	RegisterMappings ModbusRegisterMap `mapstructure:"Mappings"`
	PollingInterval  time.Duration     `mapstructure:"PollingInterval"`
}

type NMEA0183Config struct {
	Name     string `mapstructure:"Name"`
	Context  string `mapstructure:"Context"`
	URI      string `mapstructure:"URI"`
	Listen   bool   `mapstructure:"Listen"`
	Baudrate int    `mapstructure:"Baudrate"`
}

func NewModbusConfig(configFilePath string) ModbusConfig {
	var result ModbusConfig
	var modbusEnv = map[string]interface{}{
		"registers": []uint16{},
	}

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

	for _, registerMapping := range result.RegisterMappings {
		if registerMapping.Expression == "" || len(registerMapping.SignalKPath) == 0 {
			continue
		}
		program, err := expr.Compile(registerMapping.Expression, expr.Env(modbusEnv))
		if err != nil {
			logger.GetLogger().Warn(
				"Could not compile the mapping function",
				zap.String("Mapping function", registerMapping.Expression),
				zap.String("Error", err.Error()),
			)
			continue
		}
		registerMapping.CompiledExpression = program
	}

	return result
}

func NewNMEA0183Config(configFilePath string) NMEA0183Config {
	var result NMEA0183Config

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
