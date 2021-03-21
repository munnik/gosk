package config

import (
	"time"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var modbusEnv = map[string]interface{}{
	"registers": []uint16{},
}

type ModbusRegister struct {
	FunctionCode       uint16   `mapstructure:"FunctionCode"`
	Size               uint16   `mapstructure:"Size"`
	SignalKPath        []string `mapstructure:"SignalKPath"`
	Expression         string   `mapstructure:"Expression"`
	CompiledExpression *vm.Program
}

type ModbusConfig struct {
	RegisterMappings map[uint16]*ModbusRegister `mapstructure:"Mappings"`
	Context          string                     `mapstructure:"Context"`
	PollingInterval  time.Duration              `mapstructure:"PollingInterval"`
}

func NewModbusConfig(configFilePath string) ModbusConfig {
	var result ModbusConfig

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
