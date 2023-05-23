package connector

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/protocol"
	"github.com/simonvetter/modbus"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type ModbusConnector struct {
	config               *config.ConnectorConfig
	registerGroupsConfig []config.RegisterGroupConfig
	realClient           *modbus.ModbusClient
}

func NewModbusConnector(c *config.ConnectorConfig, rgcs []config.RegisterGroupConfig) (*ModbusConnector, error) {
	for _, rgc := range rgcs {
		// TODO add write function codes
		if rgc.FunctionCode == protocol.READ_COILS || rgc.FunctionCode == protocol.READ_DISCRETE_INPUTS {
			if rgc.NumberOfCoilsOrRegisters > protocol.MODBUS_MAXIMUM_NUMBER_OF_COILS {
				return nil, fmt.Errorf("maximum number %v of coils exceeded for register group %v", protocol.MODBUS_MAXIMUM_NUMBER_OF_COILS, rgc)
			}
		} else {
			if rgc.NumberOfCoilsOrRegisters > protocol.MODBUS_MAXIMUM_NUMBER_OF_REGISTERS {
				return nil, fmt.Errorf("maximum number %v of registers exceeded for register group %v", protocol.MODBUS_MAXIMUM_NUMBER_OF_REGISTERS, rgc)
			}
		}
	}
	realClient, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:      c.URL.String(),
		Speed:    uint(c.BaudRate),
		DataBits: uint(c.DataBits),
		Parity:   uint(c.Parity),
		StopBits: uint(c.StopBits),
		Timeout:  1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create modbus client %v, the error that occurred was %v", c.URL.String(), err)
	}
	if err := realClient.Open(); err != nil {
		return nil, fmt.Errorf("unable to open modbus client %v, the error that occurred was %v", c.URL.String(), err)
	}

	return &ModbusConnector{config: c, registerGroupsConfig: rgcs, realClient: realClient}, nil
}

func (m *ModbusConnector) Publish(publisher mangos.Socket) {
	stream := make(chan []byte, 1)
	defer close(stream)
	go func() {
		for {
			if err := m.receive(stream); err != nil {
				logger.GetLogger().Warn(
					"Error while receiving data for the stream",
					zap.String("URL", m.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
	process(stream, m.config.Name, m.config.Protocol, publisher)
}

func (m *ModbusConnector) Subscribe(subscriber mangos.Socket) {
	go func(connector *ModbusConnector, subscriber mangos.Socket) {
		client := protocol.NewModbusClient(
			connector.realClient,
			nil, // no need to set this because it will not be used in the Write([]byte) function
		)
		raw := &message.Raw{}
		for {
			received, err := subscriber.Recv()
			if err != nil {
				logger.GetLogger().Warn(
					"Could not receive a message from the publisher",
					zap.String("Error", err.Error()),
				)
				continue
			}
			if err := json.Unmarshal(received, raw); err != nil {
				logger.GetLogger().Warn(
					"Could not unmarshal the received data",
					zap.ByteString("Received", received),
					zap.String("Error", err.Error()),
				)
				continue
			}
			client.Write(raw.Value)
		}
	}(m, subscriber)
}

func (m *ModbusConnector) receive(stream chan<- []byte) error {
	errors := make(chan error)
	done := make(chan bool)
	var wg sync.WaitGroup
	wg.Add(len(m.registerGroupsConfig))

	// start a go routine for each register group, if an error occurs send it on the error channel
	for _, rgc := range m.registerGroupsConfig {
		go func(rgc config.RegisterGroupConfig) {
			client := protocol.NewModbusClient(m.realClient, rgc.ExtractModbusHeader())
			if err := client.Poll(stream, rgc.PollingInterval); err != nil {
				errors <- err
			}
			wg.Done()
		}(rgc)
	}
	go func() {
		// if the reading of all register groups is finished close the done channel
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// all reading is done, break the select statement
		break
	case err := <-errors:
		close(errors)
		return err
	}
	return nil
}
