package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

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
	// CSVType is used to identify the data as comma seperated values data
	CSVType = "csv"
	// JSONType is used to identify the data as json messages
	JSONType = "json"

	ParityMap string = "NOE" // None, Odd, Even
)

const (
	Coils = iota + 1
	DiscreteInputs
	HoldingRegisters
	InputRegisters
)

type CollectorConfig struct {
	Name         string   `mapstructure:"name"`
	URL          *url.URL `mapstructure:"_"`
	URLString    string   `mapstructure:"url"`
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
		ParityString: "N",
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

	result.URL, _ = url.Parse(result.URLString)
	result.Parity = strings.Index(ParityMap, result.ParityString)

	return &result
}

func (c *CollectorConfig) WithProtocol(p string) *CollectorConfig {
	c.Protocol = p
	return c
}

type RegisterGroupConfig struct {
	Slave                  uint8         `mapstructure:"slave"`
	FunctionCode           uint16        `mapstructure:"functionCode"`
	Address                uint16        `mapstructure:"address"`
	NumberOfCoilsRegisters uint16        `mapstructure:"numberOfCoilsOrRegisters"`
	PollingInterval        time.Duration `mapstructure:"pollingInterval"`
}

func NewRegisterGroupsConfig(configFilePath string) []RegisterGroupConfig {
	var result []RegisterGroupConfig
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.UnmarshalKey("registerGroups", &result)
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

type ModbusMappingsConfig struct {
	Slave                    uint8  `mapstructure:"slave"`
	FunctionCode             uint16 `mapstructure:"functionCode"`
	Address                  uint16 `mapstructure:"address"`
	NumberOfCoilsOrRegisters uint16 `mapstructure:"numberOfCoilsOrRegisters"`
	Expression               string `mapstructure:"expression"`
	CompiledExpression       *vm.Program
	Path                     string `mapstructure:"path"`
}

func NewModbusMappingsConfig(configFilePath string) []ModbusMappingsConfig {
	var result []ModbusMappingsConfig
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.UnmarshalKey("mappings", &result)
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}

	for _, rmc := range result {
		if rmc.Path == "" {
			logger.GetLogger().Warn(
				"Path was not set",
				zap.String("Register mapping", fmt.Sprintf("%+v", rmc)),
			)
			continue
		}
		if rmc.Expression == "" {
			logger.GetLogger().Warn(
				"Expression was not set",
				zap.String("Register mapping", fmt.Sprintf("%+v", rmc)),
			)
		}
	}
	return result
}

type CSVMappingConfig struct {
	BeginsWith         string `mapstructure:"beginsWith"`
	Expression         string `mapstructure:"expression"`
	CompiledExpression *vm.Program
	Path               string `mapstructure:"path"`
}

func NewCSVMappingConfig(configFilePath string) []CSVMappingConfig {
	var result []CSVMappingConfig
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.UnmarshalKey("mappings", &result)
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}

	for _, cmc := range result {
		if cmc.Path == "" {
			logger.GetLogger().Warn(
				"Path was not set",
				zap.String("Register mapping", fmt.Sprintf("%+v", cmc)),
			)
			continue
		}
		if cmc.Expression == "" {
			logger.GetLogger().Warn(
				"Expression was not set",
				zap.String("Register mapping", fmt.Sprintf("%+v", cmc)),
			)
		}
	}
	return result
}

type JSONMappingConfig struct {
	Expression         string `mapstructure:"expression"`
	CompiledExpression *vm.Program
	Path               string `mapstructure:"path"`
}

func NewJSONMappingConfig(configFilePath string) []JSONMappingConfig {
	var result []JSONMappingConfig
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	err := viper.UnmarshalKey("mappings", &result)
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}

	for _, jmc := range result {
		if jmc.Path == "" {
			logger.GetLogger().Warn(
				"Path was not set",
				zap.String("Register mapping", fmt.Sprintf("%+v", jmc)),
			)
			continue
		}
		if jmc.Expression == "" {
			logger.GetLogger().Warn(
				"Expression was not set",
				zap.String("Register mapping", fmt.Sprintf("%+v", jmc)),
			)
		}
	}
	return result
}

type MQTTConfig struct {
	URLString string `mapstructure:"url"`
	ClientId  string `mapstructure:"client_id"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	Context   string `mapstructure:"context"`
	Interval  int    `mapstructure:"interval"`
}

func NewMQTTConfig(configFilePath string) *MQTTConfig {
	result := MQTTConfig{}
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

	return &result
}

type CacheConfig struct {
	LifeWindow       int    `mapstructure:"lifeWindow"`       // time after which entry can be evicted, value in seconds
	HardMaxCacheSize int    `mapstructure:"hardMaxCacheSize"` // cache will not allocate more memory than this limit, value in MB
	Heartbeat        uint64 `mapstructure:"heartbeat"`        // every heartbeat all cached values that are in the cache for at least one heartbeat will be send again, value in seconds
}

type PostgresqlConfig struct {
	URLString string `mapstructure:"url"`
}

func NewPostgresqlConfig(configFilePath string) *PostgresqlConfig {
	result := PostgresqlConfig{}
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

	return &result
}
