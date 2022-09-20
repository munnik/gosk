package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/logger"
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

	CanBusType = "canbus"

	SignalKType = "signalk"

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

type CanBusMapperConfig struct {
	MapperConfig `mapstructure:",squash"`
	DbcFile      string `mapstructure:"dbcFile"`
}

func NewCanBusMapperConfig(configFilePath string) CanBusMapperConfig {
	result := CanBusMapperConfig{}
	readConfigFile(&result, configFilePath)

	return result
}

type MappingConfig struct {
	Expression         string `mapstructure:"expression"`
	CompiledExpression *vm.Program
	Path               string `mapstructure:"path"`
}

func (m *MappingConfig) verify() {
	if m.Path == "" {
		logger.GetLogger().Warn(
			"Path was not set",
			zap.String("Register mapping", fmt.Sprintf("%+v", m)),
		)
	}
	if m.Expression == "" {
		logger.GetLogger().Warn(
			"Expression was not set",
			zap.String("Register mapping", fmt.Sprintf("%+v", m)),
		)
	}
}

type ModbusMappingsConfig struct {
	MappingConfig            `mapstructure:",squash"`
	Slave                    uint8  `mapstructure:"slave"`
	FunctionCode             uint16 `mapstructure:"functionCode"`
	Address                  uint16 `mapstructure:"address"`
	NumberOfCoilsOrRegisters uint16 `mapstructure:"numberOfCoilsOrRegisters"`
}

func NewModbusMappingsConfig(configFilePath string) []ModbusMappingsConfig {
	var result []ModbusMappingsConfig
	readConfigFile(&result, configFilePath, "mappings")
	for _, rmc := range result {
		rmc.verify()
	}
	return result
}

type CSVMappingConfig struct {
	BeginsWith    string `mapstructure:"beginsWith"`
	MappingConfig `mapstructure:",squash"`
}

func NewCSVMappingConfig(configFilePath string) []CSVMappingConfig {
	var result []CSVMappingConfig
	readConfigFile(&result, configFilePath, "mappings")

	for _, rmc := range result {
		rmc.verify()
	}
	return result
}

type JSONMappingConfig struct {
	MappingConfig `mapstructure:",squash"`
}

func NewJSONMappingConfig(configFilePath string) []JSONMappingConfig {
	var result []JSONMappingConfig
	readConfigFile(&result, configFilePath, "mappings")

	for _, rmc := range result {
		rmc.verify()
	}
	return result
}

type AggegrateMappingConfig struct {
	MappingConfig `mapstructure:",squash"`
	SourcePaths   []string `mapstructure:"sourcePaths"`
}

func NewAggegrateMappingConfig(configFilePath string) []AggegrateMappingConfig {
	var result []AggegrateMappingConfig
	readConfigFile(&result, configFilePath, "mappings")

	for _, rmc := range result {
		rmc.verify()
	}
	return result
}

type CanBusMappingConfig struct {
	MappingConfig `mapstructure:",squash"`
	Name          string `mapstructure:"name"`
	Origin        string `mapstructure:"origin"`
}

func NewCanBusMappingConfig(configFilePath string) []CanBusMappingConfig {
	var result []CanBusMappingConfig
	readConfigFile(&result, configFilePath, "mappings")
	for _, rmc := range result {
		rmc.verify()
	}
	return result
}

type MQTTConfig struct {
	URLString        string `mapstructure:"url"`
	Username         string `mapstructure:"username"`
	Password         string `mapstructure:"password"`
	Interval         int    `mapstructure:"interval"`         // interval to flush the cache in seconds, ignored for reader
	HardMaxCacheSize int    `mapstructure:"hardMaxCacheSize"` // maximum size of the cache in MBs, cache will be flushed when size is reached, ignored for reader
}

func NewMQTTConfig(configFilePath string) *MQTTConfig {
	result := MQTTConfig{}
	readConfigFile(&result, configFilePath)

	return &result
}

type PostgresqlConfig struct {
	URLString          string  `mapstructure:"url"`
	BatchSize          int     `mapstructure:"batch_size"`
	BatchFlushInterval int     `mapstructure:"batch_flush_interval"`
	CompleteRatio      float64 `mapstructure:"complete_ratio" default:"1.00"` // a period is considered complete when local / remote >= CompleteRatio
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
	PostgresqlConfig *PostgresqlConfig `mapstructure:"database"`
	BigCacheConfig   *BigCacheConfig   `mapstructure:"cache"`
}

func NewSignalKConfig(configFilePath string) *SignalKConfig {
	result := SignalKConfig{Version: "undefined"}
	readConfigFile(&result, configFilePath)

	result.URL, _ = url.Parse(result.URLString)

	return &result
}

func (c *SignalKConfig) WithVersion(version string) *SignalKConfig {
	c.Version = version
	return c
}

type OriginsConfig struct {
	Origin string    `mapstructure:"origin"`
	Epoch  time.Time `mapstructure:"epoch"`
}

type TransferConfig struct {
	PostgresqlConfig PostgresqlConfig `mapstructure:"database"`
	MQTTConfig       MQTTConfig       `mapstructure:"mqtt"`
	Origin           string           `mapstructure:"origin"`
	Origins          []OriginsConfig  `mapstructure:"origins"`
}

func NewTransferConfig(configFilePath string) *TransferConfig {
	result := &TransferConfig{}
	readConfigFile(result, configFilePath)

	return result
}

type LWEConfig struct {
	DestinationIdentification string `mapstructure:"destination_identification"`
	SourceIdentification      string `mapstructure:"source_identification"`
	IncludeTimestamp          bool   `mapstructure:"include_timestamp"`
	IncludeLineCount          bool   `mapstructure:"include_line_count"`
}

func NewLWEConfig(configFilePath string) *LWEConfig {
	result := &LWEConfig{}
	readConfigFile(result, configFilePath)

	return result
}

type EventConfig struct {
	Expression string `mapstructure:"expression"`
}

func NewEventConfig(configFilePath string) *EventConfig {
	result := &EventConfig{}
	readConfigFile(result, configFilePath)

	return result
}

type RateLimitsConfig struct {
	Path     string        `mapstructure:"path"`
	Interval time.Duration `mapstructure:"interval"`
}

type RateLimitConfig struct {
	Ratelimits      []RateLimitsConfig `mapstructure:"rateLimits"`
	DefaultInterval time.Duration      `mapstructure:"defaultInterval" default:"1s"`
}

func NewRateLimitConfig(configFilePath string) *RateLimitConfig {
	result := &RateLimitConfig{}
	readConfigFile(result, configFilePath)

	return result
}
