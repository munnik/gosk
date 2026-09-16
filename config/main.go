package config

import (
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/expr-lang/expr/vm"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/protocol"
	"go.uber.org/zap"
)

const (
	// NMEA0183Type is used to identify the data as NMEA 0183 data
	NMEA0183Type = "nmea0183"
	// ModbusType is used to identify the data as Modbus data
	ModbusType = "modbus"
	// CSVType is used to identify the data as comma separated values data
	CSVType = "csv"
	// JSONType is used to identify the data as json messages
	JSONType = "json"

	CanBusType = "canbus"

	SignalKType = "signalk"

	HttpType = "http"

	MannerEthernetType = "manner_ethernet"

	BinaryType = "binary"

	MQTTType = "mqtt"

	FftType = "fft"

	ParityMap string = "NOE" // None, Odd, Even
)

type ConnectorConfig struct {
	Name      string        `mapstructure:"name"`
	URL       *url.URL      `mapstructure:"_"`
	URLString string        `mapstructure:"url"`
	Listen    bool          `mapstructure:"listen"`
	BaudRate  int           `mapstructure:"baudRate"`
	DataBits  int           `mapstructure:"dataBits"`
	StopBits  string        `mapstructure:"stopBits"`
	Parity    string        `mapstructure:"parity"`
	Protocol  string        `mapstructure:"protocol"`
	Timeout   time.Duration `mapstructure:"timeout"`
}

func NewConnectorConfig(configFilePath string) *ConnectorConfig {
	result := &ConnectorConfig{
		Listen:   false,
		BaudRate: 4800,
		DataBits: 8,
		StopBits: "1",
		Parity:   "N",
		Timeout:  5 * time.Minute,
	}
	readConfigFile(result, configFilePath)

	result.URL, _ = url.Parse(result.URLString)

	return result
}

type RegisterGroupConfig struct {
	protocol.ModbusHeader `mapstructure:",squash"`
	PollingInterval       time.Duration      `mapstructure:"pollingInterval"`
	WriteBeforeRead       ModbusWriteRequest `mapstructure:"writeBeforeRead"`
}

func NewRegisterGroupsConfig(configFilePath string) []RegisterGroupConfig {
	var result []RegisterGroupConfig
	readConfigFile(&result, configFilePath, "registerGroups")

	return result
}

func (rgc *RegisterGroupConfig) ExtractModbusHeader() *protocol.ModbusHeader {
	return &protocol.ModbusHeader{
		Slave:                    rgc.Slave,
		FunctionCode:             rgc.FunctionCode,
		Address:                  rgc.Address,
		NumberOfCoilsOrRegisters: rgc.NumberOfCoilsOrRegisters,
	}
}

func (rgc *RegisterGroupConfig) ExtractWriteModbusHeader() *protocol.ModbusHeader {
	if rgc.WriteBeforeRead.IsEmpty() {
		return nil
	}
	return &protocol.ModbusHeader{
		Slave:                    rgc.WriteBeforeRead.Slave,
		FunctionCode:             rgc.WriteBeforeRead.FunctionCode,
		Address:                  rgc.WriteBeforeRead.Address,
		NumberOfCoilsOrRegisters: rgc.WriteBeforeRead.NumberOfCoilsOrRegisters,
	}
}

type ModbusWriteRequest struct {
	protocol.ModbusHeader `mapstructure:",squash"`
	Values                []byte        `mapstructure:"values"`
	Delay                 time.Duration `mapstructure:"delay"`
}

func (mwr *ModbusWriteRequest) IsEmpty() bool {
	return mwr.Slave == 0 && mwr.FunctionCode == 0 && mwr.Address == 0 && mwr.NumberOfCoilsOrRegisters == 0 && len(mwr.Values) == 0
}

type UrlGroupConfig struct {
	Url             string        `mapstructure:"url"`
	PollingInterval time.Duration `mapstructure:"pollingInterval"`
}

func NewUrlGroupsConfig(configFilePath string) []UrlGroupConfig {
	var result []UrlGroupConfig
	readConfigFile(&result, configFilePath, "urlGroups")

	return result
}

const (
	ProtocolOptionNmeaParse                = "nmeaparse"
	ProtocolOptionModbusSkipFaultDetection = "skipfaultdetection"
)

type MapperConfig struct {
	Context         string            `mapstructure:"context"`
	Protocol        string            `mapstructure:"protocol"`
	ProtocolOptions map[string]string `mapstructure:"protocolOptions"`
	Interval        time.Duration     `mapstructure:"interval"`
}

func NewMapperConfig(configFilePath string) MapperConfig {
	result := MapperConfig{}
	readConfigFile(&result, configFilePath)

	return result
}

type CanBusMapperConfig struct {
	MapperConfig `mapstructure:",squash"`
	DbcFile      string `mapstructure:"dbcFile"`
	IsJ1939      bool   `mapstructure:"isJ1939"`
}

func NewCanBusMapperConfig(configFilePath string) CanBusMapperConfig {
	result := CanBusMapperConfig{}
	readConfigFile(&result, configFilePath)

	return result
}

type CSVMapperConfig struct {
	MapperConfig `mapstructure:",squash"`
	Separator    string `mapstructure:"separator"`
	SplitLines   bool   `mapstructure:"splitLines"`
}

func NewCSVMapperConfig(configFilePath string) CSVMapperConfig {
	result := CSVMapperConfig{SplitLines: false, Separator: ","}
	readConfigFile(&result, configFilePath)

	return result
}

type MappingConfig struct {
	Expression                  string                 `mapstructure:"expression"`
	TimestampExpression         string                 `mapstructure:"timestampExpression"`
	ExpressionEnvironment       map[string]interface{} `mapstructure:"expressionEnvironment"`
	CompiledExpression          *vm.Program
	CompiledTimestampExpression *vm.Program
	Path                        string `mapstructure:"path"`
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
	MappingConfig         `mapstructure:",squash"`
	protocol.ModbusHeader `mapstructure:",squash"`
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
	MappingConfig `mapstructure:",squash"`
	BeginsWith    string `mapstructure:"beginsWith"`
	Regex         string `mapstructure:"regex"`
	ReplaceWith   string `mapstructure:"replaceWith"`
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

type ExpressionMappingConfig struct {
	MappingConfig `mapstructure:",squash"`
	SourcePaths   []string      `mapstructure:"sourcePaths"`
	RetentionTime time.Duration `mapstructure:"retentionTime"`
	Overwrite     bool          `mapstructure:"overwrite"`
}

func NewExpressionMappingConfig(configFilePath string) []*ExpressionMappingConfig {
	var result []*ExpressionMappingConfig
	readConfigFile(&result, configFilePath, "mappings")

	for _, rmc := range result {
		rmc.verify()
	}
	return result
}

// NotificationMappingConfig describes a single notification check on mapped
// data. Expression is a boolean expression, e.g. comparing a ratio or a
// difference between two paths against expected bounds. It evaluates to
// true when the check's notification should be raised and false when it
// should be cleared. When is an optional boolean expression, the check is
// only performed when it evaluates to true, it defaults to always
// performing the check.
//
// To avoid flapping notifications, raising the notification is only
// confirmed once the expression has evaluated to true continuously for
// SetDelay, and clearing it is only confirmed once the expression has
// evaluated to false continuously for ResetDelay. Both default to zero,
// i.e. confirmed immediately, when omitted.
//
// State and Method map onto the Signal K notification object's state and
// method properties, and are only used while the notification is raised,
// Message and State are required. Clearing the notification publishes a
// null value instead, as described at
// https://signalk.org/specification/1.8.2/doc/notifications.html. Method
// defaults to ["sound", "visual"] when omitted.
//
// Timeout, when set, additionally treats a source path as gone once it has
// not received a new value for longer than Timeout, and fails safe by
// assuming the notification is needed, the same way a failed Expression
// does. It only applies to a source path that has been seen at least once,
// a source path that never reported a value is left to Expression to fail
// on. Timeout is disabled (the default) when omitted.
type NotificationMappingConfig struct {
	MappingConfig `mapstructure:",squash"`
	SourcePaths   []string `mapstructure:"sourcePaths"`
	When          string   `mapstructure:"when"`
	CompiledWhen  *vm.Program
	Message       string        `mapstructure:"message"`
	State         string        `mapstructure:"state"`
	Method        []string      `mapstructure:"method"`
	SetDelay      time.Duration `mapstructure:"setDelay"`
	ResetDelay    time.Duration `mapstructure:"resetDelay"`
	Timeout       time.Duration `mapstructure:"timeout"`
}

func NewNotificationMappingConfig(configFilePath string) []*NotificationMappingConfig {
	var result []*NotificationMappingConfig
	readConfigFile(&result, configFilePath, "mappings")
	for _, nmc := range result {
		nmc.verify()
	}
	return result
}

func (n *NotificationMappingConfig) verify() {
	// notificationStates are the valid values for a Signal K notification's
	// state property, see
	// https://signalk.org/specification/1.8.2/doc/notifications.html
	notificationStates := []string{"nominal", "normal", "alert", "warn", "alarm", "emergency"}

	n.MappingConfig.verify()
	if n.Message == "" {
		logger.GetLogger().Warn(
			"Message was not set",
			zap.String("Notification mapping", fmt.Sprintf("%+v", n)),
		)
	}
	if n.State == "" {
		logger.GetLogger().Warn(
			"State was not set",
			zap.String("Notification mapping", fmt.Sprintf("%+v", n)),
		)
	} else if !slices.Contains(notificationStates, n.State) {
		logger.GetLogger().Warn(
			"State is not a valid Signal K notification state",
			zap.String("State", n.State),
			zap.Strings("Valid states", notificationStates),
		)
	}
	if len(n.Method) == 0 {
		n.Method = []string{"sound", "visual"}
	}
}

type FftConfig struct {
	MappingConfig         `mapstructure:",squash"`
	Path                  string  `mapstructure:"path"`
	SpectrumPath          string  `mapstructure:"spectrumPath"`
	SamplesChannelBitSize int64   `mapstructure:"samplesChannelBitSize"`
	FrequencyStepSize     float64 `mapstructure:"frequencyStepSize"`
	// GapDetectionThreshold flags an FFT window as unreliable when one
	// sample interval in it is more than this many times the window's mean
	// interval [optional default is 3]
	GapDetectionThreshold float64 `mapstructure:"gapDetectionThreshold"`
}

func NewFftConfig(configFilePath string) []*FftConfig {
	var result []*FftConfig
	readConfigFile(&result, configFilePath, "mappings")
	for _, fftc := range result {
		fftc.verify()
	}
	return result
}

type CanBusMappingConfig struct {
	MappingConfig `mapstructure:",squash"`
	Name          string    `mapstructure:"name"`
	Origin        string    `mapstructure:"origin"`
	ExcludeValues []float64 `mapstructure:"excludeValues"`
}

func NewCanBusMappingConfig(configFilePath string) []CanBusMappingConfig {
	var result []CanBusMappingConfig
	readConfigFile(&result, configFilePath, "mappings")
	for _, rmc := range result {
		rmc.verify()
	}
	return result
}

func NewMappingConfig(configFilePath string) []MappingConfig {
	var result []MappingConfig
	readConfigFile(&result, configFilePath, "mappings")
	for _, rmc := range result {
		rmc.verify()
	}
	return result
}

type MQTTConfig struct {
	URLString  string        `mapstructure:"url"`
	Username   string        `mapstructure:"username"`
	Password   string        `mapstructure:"password"`
	Interval   time.Duration `mapstructure:"interval"`    // interval to flush the cache in seconds, ignored for reader
	BufferSize int           `mapstructure:"buffer_size"` // maximum size of the cache in MBs, cache will be flushed when size is reached, ignored for reader
	Compress   bool          `mapstructure:"compress"`    // compress the data before sending
	Topic      string        `mapstructure:"topic"`       // topic to subscribe to
}

func NewMQTTConfig(configFilePath string) *MQTTConfig {
	result := MQTTConfig{
		BufferSize: 100,
		Interval:   30 * time.Second,
		Compress:   true,
	}
	readConfigFile(&result, configFilePath)

	return &result
}

type PostgresqlConfig struct {
	URLString          string        `mapstructure:"url"`
	BatchFlushLength   int           `mapstructure:"batch_flush_length"`
	BatchFlushInterval time.Duration `mapstructure:"batch_flush_interval"`
	BufferSize         int           `mapstructure:"buffer_size"`       // size of the buffer for incoming messages
	NumberOfWorkers    int           `mapstructure:"number_of_workers"` // number of workers to handle the incoming messages
	Timeout            time.Duration `mapstructure:"timeout"`
}

func defaultPostgresqlConfig() PostgresqlConfig {
	return PostgresqlConfig{
		BufferSize:         100,
		NumberOfWorkers:    10,
		Timeout:            5 * time.Second,
		BatchFlushLength:   100,
		BatchFlushInterval: 10 * time.Second,
	}
}

func NewPostgresqlConfig(configFilePath string) *PostgresqlConfig {
	result := defaultPostgresqlConfig()
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
	postgres := defaultPostgresqlConfig()
	result := SignalKConfig{
		Version:          "undefined",
		PostgresqlConfig: &postgres,
	}
	readConfigFile(&result, configFilePath)

	result.URL, _ = url.Parse(result.URLString)

	return &result
}

func (c *SignalKConfig) WithVersion(version string) *SignalKConfig {
	c.Version = version
	return c
}

type TransferConfig struct {
	PostgresqlConfig          PostgresqlConfig `mapstructure:"database"`
	MQTTConfig                MQTTConfig       `mapstructure:"mqtt"`
	Origin                    string           `mapstructure:"origin"`
	SleepBetweenCountRequests time.Duration    `mapstructure:"sleep_between_count_requests"`
	SleepBetweenDataRequests  time.Duration    `mapstructure:"sleep_between_data_requests"`
	SleepBetweenRespondDeltas time.Duration    `mapstructure:"sleep_between_respond_deltas"`
	NumberOfRequestWorkers    int              `mapstructure:"number_of_request_workers"`
	MaxPeriodsToRequest       int              `mapstructure:"max_periods_to_request"`
	CompletenessFactor        float64          `mapstructure:"completeness_factor"`
}

func NewTransferConfig(configFilePath string) *TransferConfig {
	result := &TransferConfig{
		PostgresqlConfig:          defaultPostgresqlConfig(),
		SleepBetweenCountRequests: 30 * time.Minute,
		SleepBetweenDataRequests:  6 * time.Hour,
		SleepBetweenRespondDeltas: 10 * time.Millisecond,
		NumberOfRequestWorkers:    5,
		MaxPeriodsToRequest:       500,
		CompletenessFactor:        0.99,
	}
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

type RateLimitFilterConfig struct {
	Ratelimits      []RateLimitsConfig `mapstructure:"rateLimits"`
	DefaultInterval time.Duration      `mapstructure:"defaultInterval"`
}

func NewRateLimitConfig(configFilePath string) *RateLimitFilterConfig {
	result := &RateLimitFilterConfig{}
	readConfigFile(result, configFilePath)

	return result
}

type TestDataConfig struct {
	Context string          `mapstructure:"context"`
	Delay   time.Duration   `mapstructure:"delay"`
	Paths   []MappingConfig `mapstructure:"paths"`
}

func NewTestDataConfig(configFilePath string) *TestDataConfig {
	result := &TestDataConfig{}
	readConfigFile(result, configFilePath)

	return result
}
