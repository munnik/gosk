package collector

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper"
	"github.com/simonvetter/modbus"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// MaximumNumberOfRegisters is maximum number of registers that can be read in one request, a modbus message is limit to 256 bytes
const MaximumNumberOfRegisters = 120

// ModbusNetworkCollector collects Modbus data over a tcp connection
type ModbusNetworkCollector struct {
	URL             *url.URL
	Name            string
	RegisterRanges  []ModbusRegisterRange
	PollingInterval int
}

// ModbusRegisterRange contains all data to request one modbus register range
type ModbusRegisterRange struct {
	FunctionCode  uint8
	StartRegister uint16
	RegisterCount uint16
}

// NewModbusNetworkCollector creates an instance of a TCP collector
func NewModbusNetworkCollector(url *url.URL, name string, registerStrings []string, pollingInterval int) *ModbusNetworkCollector {
	return &ModbusNetworkCollector{
		URL:             url,
		Name:            name,
		RegisterRanges:  toModbusRegisterRange(registerStrings),
		PollingInterval: pollingInterval,
	}
}

func toModbusRegisterRange(registerStrings []string) []ModbusRegisterRange {
	result := make([]ModbusRegisterRange, 0)
	for _, registerString := range registerStrings {
		splits := strings.SplitN(registerString, ":", 3)
		functionCode, err := strconv.ParseInt(splits[0], 0, 0)
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not parse modbus register",
				zap.String("Provided register", registerString),
			)
		}
		startRegister, err := strconv.ParseInt(splits[1], 0, 0)
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not parse modbus register",
				zap.String("Provided register", registerString),
			)
		}
		registerCount, err := strconv.ParseInt(splits[2], 0, 0)
		if err != nil {
			logger.GetLogger().Fatal(
				"Could not parse modbus register",
				zap.String("Provided register", registerString),
			)
		}

		for registerCount > 0 {
			if registerCount > MaximumNumberOfRegisters {
				result = append(result, ModbusRegisterRange{
					FunctionCode:  uint8(functionCode),
					StartRegister: uint16(startRegister),
					RegisterCount: uint16(MaximumNumberOfRegisters),
				})
				startRegister += MaximumNumberOfRegisters
				registerCount -= MaximumNumberOfRegisters
			} else {
				result = append(result, ModbusRegisterRange{
					FunctionCode:  uint8(functionCode),
					StartRegister: uint16(startRegister),
					RegisterCount: uint16(registerCount),
				})
				registerCount = 0
			}
		}
	}
	return result
}

// Collect start the collection process and keeps running as long as there is data available
func (c *ModbusNetworkCollector) Collect(socket mangos.Socket) {
	stream := make(chan []byte, 1)

	go c.receive(stream)
	processStream(stream, mapper.ModbusType, socket, c.Name)
}

func (c *ModbusNetworkCollector) receive(stream chan<- []byte) error {
	defer close(stream)

	logger.GetLogger().Info(
		"Start to collect Modbus data from the network",
		zap.String("Host", c.URL.Hostname()),
		zap.String("Port", c.URL.Port()),
	)
	client, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:     c.URL.String(),
		Timeout: 1 * time.Second,
	})
	if err != nil {
		logger.GetLogger().Fatal(
			"Unable to connect to modbus slave",
			zap.String("Host", c.URL.Hostname()),
			zap.String("Port", c.URL.Port()),
			zap.String("Error", err.Error()),
		)
	}
	if err = client.Open(); err != nil {
		logger.GetLogger().Fatal(
			"Unable to connect to modbus slave",
			zap.String("Host", c.URL.Hostname()),
			zap.String("Port", c.URL.Port()),
			zap.String("Error", err.Error()),
		)
	}

	ticker := time.NewTicker(time.Duration(c.PollingInterval) * time.Millisecond)
	quitChannel := make(chan struct{})
	for {
		select {
		case <-ticker.C:
			for _, registerRange := range c.RegisterRanges {
				var registerType modbus.RegType
				if registerRange.FunctionCode == 0x03 {
					registerType = modbus.HOLDING_REGISTER
				} else if registerRange.FunctionCode == 0x04 {
					registerType = modbus.INPUT_REGISTER
				} else {
					logger.GetLogger().Warn(
						"Function code is not supported",
						zap.Uint8("Function code", registerRange.FunctionCode),
					)
					continue
				}
				result, err := client.ReadRegisters(
					registerRange.StartRegister,
					registerRange.RegisterCount,
					registerType,
				)
				if err != nil {
					logger.GetLogger().Warn(
						"Could not read Modbus registers",
						zap.Uint16("Start register", registerRange.StartRegister),
						zap.Uint16("Count", registerRange.RegisterCount),
						zap.Uint8("Function code", registerRange.FunctionCode),
					)
					continue
				}
				stream <- uint16sToBytes(result)
			}
		case <-quitChannel:
			ticker.Stop()
			return nil
		}
	}
}
