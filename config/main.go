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

	MeteoHydroType = "meteohydro"

	ParityMap string = "NOE" // None, Odd, Even
)

type ConnectorConfig struct {
	Name      string   `mapstructure:"name"`
	URL       *url.URL `mapstructure:"_"`
	URLString string   `mapstructure:"url"`
	Listen    bool     `mapstructure:"listen"`
	BaudRate  int      `mapstructure:"baudRate"`
	DataBits  int      `mapstructure:"dataBits"`
	StopBits  string   `mapstructure:"stopBits"`
	Parity    string   `mapstructure:"parity"`
	Protocol  string   `mapstructure:"protocol"`
	// Timeout is how long connector/main.go's process waits without any
	// data before reporting DisconnectedOrNoData (repeating that report
	// every Timeout for as long as it stays true - see process's doc
	// comment). Keep this under gosk.nix's systemd TimeoutStartSec (40s)
	// for every connect-type processor: systemd only considers a
	// Type=notify unit "started" once it publishes something at all, and
	// a Timeout longer than that leaves it stuck "activating" - and
	// deploy-rs/nixos-rebuild switch failing the whole fleet's deploy
	// over it - for the gap between the two.
	Timeout time.Duration `mapstructure:"timeout"`
}

func NewConnectorConfig(configFilePath string) *ConnectorConfig {
	result := &ConnectorConfig{
		Listen:   false,
		BaudRate: 4800,
		DataBits: 8,
		StopBits: "1",
		Parity:   "N",
		Timeout:  30 * time.Second,
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
	Timeout            time.Duration `mapstructure:"timeout"`
}

func defaultPostgresqlConfig() PostgresqlConfig {
	return PostgresqlConfig{
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

// MeteoHydroMapperConfig configures the meteo/hydro mapper, see
// mapper.MeteoHydroMapper. Every duration and threshold has a sane default,
// a configuration that only sets context and protocol is enough to run it
// against the public Open-Meteo API.
//
// URL is the base URL of an Open-Meteo compatible forecast endpoint and
// MarineURL that of the matching marine endpoint, the query parameters
// are added by the mapper itself. Both default to the public API, point
// them at a self hosted instance to avoid depending on (and being rate
// limited by) a third party - note that the public API is licensed for
// non-commercial use only. An empty MarineURL turns the sea state off
// entirely, which is what a fleet that never leaves the inland waterways
// wants.
//
// Models and MarineModels select the weather model to use, empty (the
// default) lets the API pick the highest resolution model that covers the
// vessel's position, which across north western Europe means KNMI
// Harmonie, Meteo-France AROME or DWD ICON-D2 depending on the country.
//
// The publish rate is max(MinPublishInterval, the rate at which navigation
// data arrives) and, while the vessel is not moving, at least
// StationaryPublishInterval. MinPublishInterval is floored at
// MinAllowedPublishInterval: weather over a single grid cell simply does
// not change faster than that, and publishing quicker only adds noise to
// every downstream sink.
//
// The weather APIs themselves are polled far less often than the mapper
// publishes: an observation is reused from the cache for the whole of
// RefreshInterval (MarineRefreshInterval for the sea state, which changes
// slower still and defaults to RefreshInterval), and only a vessel that
// leaves its grid cell (see GridResolution) causes an earlier request.
//
// InlandRetryInterval is how long a position with no sea state at all is
// remembered as such. The marine API answers with nulls everywhere for a
// position on an inland waterway, and a canal does not grow a sea state
// in ten minutes, so that answer is kept far longer than a real one - a
// barge on the Rhine would otherwise spend a request per refresh
// interval, for its whole working life, confirming that the Rhine still
// has no swell. MinRequestInterval is the
// hard floor between two requests regardless of position, failed requests
// back off exponentially from it. A cached observation older than
// MaxDataAge is not published at all, a vessel that loses internet
// connectivity goes silent instead of reporting hours old weather as if
// it were current.
//
// NavigationTimeout is how long a navigation value stays usable after it
// was last received, a position older than this stops the mapper from
// publishing anything, and a stale heading/course/speed is treated as
// absent when deciding what can be calculated.
//
// LiveDataTimeout is how long a value from another source keeps this
// mapper off that path. A modelled value is never as good as an
// instrument on the vessel itself, so any path another source is
// currently publishing is left alone entirely - which is what lets the
// mapper run on a vessel that does have a wind sensor. A source that goes
// quiet for LiveDataTimeout hands the path back rather than leaving it
// empty. For that to work the mapper has to see every other mapper on the
// vessel, see subscribeTo in the nix configuration.
//
// MinSpeedOverGround is the speed under which the vessel counts as not
// moving: its course over ground is meaningless at (almost) zero speed,
// so it is neither used as the reference direction for wind angles nor
// as the vessel's motion vector below this speed.
type MeteoHydroMapperConfig struct {
	MapperConfig              `mapstructure:",squash"`
	URL                       string        `mapstructure:"url"`
	MarineURL                 string        `mapstructure:"marineUrl"`
	Models                    string        `mapstructure:"models"`
	MarineModels              string        `mapstructure:"marineModels"`
	MinPublishInterval        time.Duration `mapstructure:"minPublishInterval"`
	StationaryPublishInterval time.Duration `mapstructure:"stationaryPublishInterval"`
	RefreshInterval           time.Duration `mapstructure:"refreshInterval"`
	MarineRefreshInterval     time.Duration `mapstructure:"marineRefreshInterval"`
	InlandRetryInterval       time.Duration `mapstructure:"inlandRetryInterval"`
	MinRequestInterval        time.Duration `mapstructure:"minRequestInterval"`
	MaxRequestInterval        time.Duration `mapstructure:"maxRequestInterval"`
	RequestTimeout            time.Duration `mapstructure:"requestTimeout"`
	MaxDataAge                time.Duration `mapstructure:"maxDataAge"`
	NavigationTimeout         time.Duration `mapstructure:"navigationTimeout"`
	LiveDataTimeout           time.Duration `mapstructure:"liveDataTimeout"`
	GridResolution            float64       `mapstructure:"gridResolution"`
	CacheSize                 int           `mapstructure:"cacheSize"`
	MinSpeedOverGround        float64       `mapstructure:"minSpeedOverGround"`
}

const (
	// MinAllowedPublishInterval is the lowest publish interval the weather
	// mapper accepts, a smaller configured MinPublishInterval is raised to
	// this.
	MinAllowedPublishInterval = 10 * time.Second
	// DefaultWeatherURL is the public Open-Meteo forecast API.
	DefaultWeatherURL = "https://api.open-meteo.com/v1/forecast"
	// DefaultMarineWeatherURL is the public Open-Meteo marine API, a
	// separate endpoint from the forecast API.
	DefaultMarineWeatherURL = "https://marine-api.open-meteo.com/v1/marine"
	// defaultGridResolution is the size, in degrees, of the cell a single
	// fetched observation is cached for, roughly 11 km of latitude.
	defaultGridResolution = 0.1
)

// DefaultMeteoHydroMapperConfig is the configuration the meteo/hydro mapper runs
// with when the configuration file only sets context and protocol.
func DefaultMeteoHydroMapperConfig() MeteoHydroMapperConfig {
	return MeteoHydroMapperConfig{
		URL:                       DefaultWeatherURL,
		MarineURL:                 DefaultMarineWeatherURL,
		MinPublishInterval:        MinAllowedPublishInterval,
		StationaryPublishInterval: 60 * time.Second,
		RefreshInterval:           10 * time.Minute,
		MarineRefreshInterval:     30 * time.Minute,
		InlandRetryInterval:       24 * time.Hour,
		MinRequestInterval:        time.Minute,
		MaxRequestInterval:        30 * time.Minute,
		RequestTimeout:            15 * time.Second,
		MaxDataAge:                3 * time.Hour,
		NavigationTimeout:         5 * time.Minute,
		LiveDataTimeout:           2 * time.Minute,
		GridResolution:            defaultGridResolution,
		CacheSize:                 64,
		MinSpeedOverGround:        0.5,
	}
}

func NewMeteoHydroMapperConfig(configFilePath string) MeteoHydroMapperConfig {
	result := DefaultMeteoHydroMapperConfig()
	readConfigFile(&result, configFilePath)
	result.verify()

	return result
}

func (w *MeteoHydroMapperConfig) verify() {
	if w.MinPublishInterval < MinAllowedPublishInterval {
		logger.GetLogger().Warn(
			"The configured minimum publish interval is too short, using the minimum allowed interval instead",
			zap.Duration("Configured", w.MinPublishInterval),
			zap.Duration("Used", MinAllowedPublishInterval),
		)
		w.MinPublishInterval = MinAllowedPublishInterval
	}
	if w.MarineRefreshInterval <= 0 {
		w.MarineRefreshInterval = w.RefreshInterval
	}
	if w.MinRequestInterval > w.RefreshInterval {
		logger.GetLogger().Warn(
			"The minimum interval between two weather API requests is longer than the refresh interval, the refresh interval is effectively the minimum request interval",
			zap.Duration("Minimum request interval", w.MinRequestInterval),
			zap.Duration("Refresh interval", w.RefreshInterval),
		)
	}
	if w.MaxRequestInterval < w.MinRequestInterval {
		w.MaxRequestInterval = w.MinRequestInterval
	}
	if w.GridResolution <= 0 {
		logger.GetLogger().Warn(
			"The grid resolution must be positive, using the default instead",
			zap.Float64("Configured", w.GridResolution),
			zap.Float64("Used", defaultGridResolution),
		)
		w.GridResolution = defaultGridResolution
	}
	if w.CacheSize < 1 {
		w.CacheSize = 1
	}
}
