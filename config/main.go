package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/logger"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	// NMEA0183Type is used to identify the data as NMEA 0183 data
	NMEA0183Type = "nmea0183"
	// ModbusType is used to identify the data as Modbus data
	ModbusType = "modbus"
	// SygoType is used to identify the data as Sygo data
	SygoType = "sygo"

	ParityMap string = "NOE" // None, Odd, Even
)

type CollectorConfig struct {
	Name         string   `mapstructure:"name"`
	URI          *url.URL `mapstructure:"_"`
	URIString    string   `mapstructure:"uri"`
	Listen       bool     `mapstructure:"listen"`
	BaudRate     int      `mapstructure:"baudRate"`
	DataBits     int      `mapstructure:"dataBits"`
	StopBits     int      `mapstructure:"stopBits"`
	Parity       int      `mapstructure:"_"`
	ParityString string   `mapstructure:"parity"`
	Protocol     string
}

func NewCollectorConfig(configFilePath string) *CollectorConfig {
	result := CollectorConfig{
		Listen:       false,
		BaudRate:     4800,
		DataBits:     8,
		StopBits:     1,
		ParityString: "E",
	}
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.UnmarshalKey("collector", &result)
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}

	result.URI, _ = url.Parse(result.URIString)
	result.Parity = strings.Index(ParityMap, result.ParityString)

	return &result
}

func (c *CollectorConfig) WithProtocol(p string) *CollectorConfig {
	c.Protocol = p
	return c
}

type RegisterGroupConfig struct {
	Slave             uint8         `mapstructure:"slave"`
	FunctionCode      uint16        `mapstructure:"functionCode"`
	Address           uint16        `mapstructure:"address"`
	NumberOfRegisters uint16        `mapstructure:"numberOfRegisters"`
	PollingInterval   time.Duration `mapstructure:"pollingInterval"`
}

func NewRegisterGroupsConfig(configFilePath string) []RegisterGroupConfig {
	var result []RegisterGroupConfig
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.UnmarshalKey("collector.registerGroups", &result)
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}
	return result
}

type MapperConfig struct {
	Context string `mapstructure:"context"`
}

func NewMapperConfig(configFilePath string) MapperConfig {
	result := MapperConfig{}
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.UnmarshalKey("mapper", &result)
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}

	return result
}

type RegisterMappingConfig struct {
	Slave              uint8  `mapstructure:"slave"`
	FunctionCode       uint16 `mapstructure:"functionCode"`
	Address            uint16 `mapstructure:"address"`
	NumberOfRegisters  uint16 `mapstructure:"numberOfRegisters"`
	Expression         string `mapstructure:"expression"`
	CompiledExpression *vm.Program
	Path               string `mapstructure:"path"`
}

func NewRegisterMappingsConfig(configFilePath string) []RegisterMappingConfig {
	var result []RegisterMappingConfig
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.UnmarshalKey("mapper.registerMappings", &result)
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}

	var env = map[string]interface{}{
		"registers": []uint16{},
	}
	for _, rm := range result {
		if rm.Path == "" {
			logger.GetLogger().Warn(
				"Path was not set",
				zap.String("Register mapping", fmt.Sprintf("%+v", rm)),
			)
			continue
		}
		if rm.Expression == "" {
			logger.GetLogger().Warn(
				"Expression was not set",
				zap.String("Register mapping", fmt.Sprintf("%+v", rm)),
			)
			continue
		}
		if rm.CompiledExpression, err = expr.Compile(rm.Expression, expr.Env(env)); err != nil {
			rm.CompiledExpression = nil
			logger.GetLogger().Warn(
				"Could not compile the mapping expression",
				zap.String("Expression", rm.Expression),
				zap.String("Error", err.Error()),
			)
		}
	}
	return result
}
