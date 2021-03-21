package collector

import (
	"net/url"
	"time"

	"github.com/munnik/gosk/config"
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
	URL    *url.URL
	Name   string
	Config config.ModbusConfig
}

// NewModbusNetworkCollector creates an instance of a TCP collector
func NewModbusNetworkCollector(url *url.URL, name string, cfg config.ModbusConfig) *ModbusNetworkCollector {
	return &ModbusNetworkCollector{
		URL:    url,
		Name:   name,
		Config: cfg,
	}
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

	ticker := time.NewTicker(c.Config.PollingInterval)
	quitChannel := make(chan struct{})
	for {
		select {
		case <-ticker.C:
			for startRegister, registerMapping := range c.Config.RegisterMappings {
				var registerType modbus.RegType
				if registerMapping.FunctionCode == 0x03 {
					registerType = modbus.HOLDING_REGISTER
				} else if registerMapping.FunctionCode == 0x04 {
					registerType = modbus.INPUT_REGISTER
				} else {
					logger.GetLogger().Warn(
						"Function code is not supported",
						zap.Uint16("Function code", registerMapping.FunctionCode),
					)
					continue
				}
				result, err := client.ReadRegisters(
					startRegister,
					registerMapping.Size,
					registerType,
				)
				if err != nil {
					logger.GetLogger().Warn(
						"Could not read Modbus registers",
						zap.Uint16("Start register", startRegister),
						zap.Uint16("Count", registerMapping.Size),
						zap.Uint16("Function code", registerMapping.FunctionCode),
					)
					continue
				}
				result = append([]uint16{registerMapping.FunctionCode, startRegister, registerMapping.Size}, result...)
				stream <- uint16sToBytes(result)
			}
		case <-quitChannel:
			ticker.Stop()
			return nil
		}
	}
}
