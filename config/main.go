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
	Protocol     string   `mapstructure:"protocol"`
}

func NewCollectorConfig(configFilePath string) *CollectorConfig {
	result := &CollectorConfig{
		Listen:       false,
		BaudRate:     4800,
		DataBits:     8,
		StopBits:     1,
		ParityString: "N",
	}
	readConfigFile(result, configFilePath)
	fmt.Println(result)

	result.URL, _ = url.Parse(result.URLString)
	result.Parity = strings.Index(ParityMap, result.ParityString)

	return result
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
	readConfigFile(&result, configFilePath, "registerGroups")

	return result
}

type MapperConfig struct {
	Context  string `mapstructure:"context"`
	Protocol string `mapstructure:"protocol"`
}

func NewMapperConfig(configFilePath string) MapperConfig {
	result := MapperConfig{}
	readConfigFile(&result, configFilePath)

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
	readConfigFile(&result, configFilePath, "mappings")

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
	readConfigFile(&result, configFilePath, "mappings")

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
	readConfigFile(&result, configFilePath, "mappings")

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
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	Interval  int    `mapstructure:"interval"`
}

func NewMQTTConfig(configFilePath string) *MQTTConfig {
	result := MQTTConfig{}
	readConfigFile(&result, configFilePath)

	return &result
}

type CacheConfig struct {
	Heartbeat      uint64          `mapstructure:"heartbeat"` // every heartbeat all cached values that are in the cache for at least one heartbeat will be send again, value in seconds
	BigCacheConfig *BigCacheConfig `mapstructure:"_"`
}

func NewCacheConfig(configFilePath string) *CacheConfig {
	result := CacheConfig{}
	readConfigFile(&result, configFilePath)
	readConfigFile(&result.BigCacheConfig, configFilePath, "cache")

	return &result
}

type PostgresqlConfig struct {
	URLString string `mapstructure:"url"`
}

func NewPostgresqlConfig(configFilePath string) *PostgresqlConfig {
	result := PostgresqlConfig{}
	readConfigFile(&result, configFilePath)

	return &result
}

type BigCacheConfig struct {
	LifeWindow       int `mapstructure:"lifeWindow"`       // time after which entry can be evicted, value in seconds
	HardMaxCacheSize int `mapstructure:"hardMaxCacheSize"` // cache will not allocate more memory than this limit, value in MB
}

func NewBigCacheConfig(configFilePath string) *BigCacheConfig {
	result := BigCacheConfig{}
	readConfigFile(&result, configFilePath)

	return &result
}

type SignalKConfig struct {
	URLString        string            `mapstructure:"url"`
	URL              *url.URL          `mapstructure:"_"`
	Version          string            `mapstructure:"_"`
	SelfContext      string            `mapstructure:"self_context"`
	PostgresqlConfig *PostgresqlConfig `mapstructure:"_"`
	BigCacheConfig   *BigCacheConfig   `mapstructure:"_"`
}

func NewSignalKConfig(configFilePath string) *SignalKConfig {
	result := SignalKConfig{Version: "undefined"}
	readConfigFile(&result, configFilePath)
	readConfigFile(&result.PostgresqlConfig, configFilePath, "db")
	readConfigFile(&result.BigCacheConfig, configFilePath, "cache")

	result.URL, _ = url.Parse(result.URLString)

	return &result
}

func (c *SignalKConfig) WithVersion(version string) *SignalKConfig {
	c.Version = version
	return c
}

func readConfigFile(cfg interface{}, configFilePath string, subKeys ...string) interface{} {
	viper.SetConfigFile(configFilePath)
	viper.ReadInConfig()

	if len(subKeys) > 1 {
		logger.GetLogger().Fatal(
			"Unable to read the configuration, only one key is allowed",
			zap.String("Config file", configFilePath),
			zap.Strings("Keys", subKeys),
		)
		return nil
	}

	var err error
	if len(subKeys) == 0 {
		err = viper.Unmarshal(cfg)
	} else {
		err = viper.UnmarshalKey(subKeys[0], cfg)
	}
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to read the configuration",
			zap.String("Config file", configFilePath),
			zap.String("Error", err.Error()),
		)
	}

	return cfg
}
