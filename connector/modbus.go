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
		if rgc.FunctionCode == protocol.ReadCoils || rgc.FunctionCode == protocol.ReadDiscreteInputs {
			if rgc.NumberOfCoilsOrRegisters > protocol.MaximumNumberOfCoils {
				return nil, fmt.Errorf("maximum number %v of coils exceeded for register group %v", protocol.MaximumNumberOfCoils, rgc)
			}
		} else {
			if rgc.NumberOfCoilsOrRegisters > protocol.MaximumNumberOfRegisters {
				return nil, fmt.Errorf("maximum number %v of registers exceeded for register group %v", protocol.MaximumNumberOfRegisters, rgc)
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
		client := NewModbusClient(
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
			client := NewModbusClient(m.realClient, rgc.ExtractModbusHeader())
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

type ModbusClient struct {
	realClient *modbus.ModbusClient
	header     *protocol.ModbusHeader
}

func NewModbusClient(realClient *modbus.ModbusClient, header *protocol.ModbusHeader) *ModbusClient {
	return &ModbusClient{
		realClient: realClient,
		header:     header,
	}
}

func (m *ModbusClient) Read(bytes []byte) (n int, err error) {
	bytes = bytes[:0]
	m.realClient.SetUnitId(m.header.Slave)
	switch m.header.FunctionCode {
	case protocol.ReadCoils:
		result, err := m.realClient.ReadCoils(m.header.Address, m.header.NumberOfCoilsOrRegisters)
		if err != nil {
			return 0, fmt.Errorf("error while reading coils %v, with length %v and function code %v, the error that occurred was %v", m.header.Address, m.header.NumberOfCoilsOrRegisters, m.header.FunctionCode, err)
		}
		bytes = append(bytes, protocol.InjectModbusHeader(m.header, protocol.CoilsToBytes(result))...)
	case protocol.ReadDiscreteInputs:
		result, err := m.realClient.ReadDiscreteInputs(m.header.Address, m.header.NumberOfCoilsOrRegisters)
		if err != nil {
			return 0, fmt.Errorf("error while reading discrete inputs %v, with length %v and function code %v, the error that occurred was %v", m.header.Address, m.header.NumberOfCoilsOrRegisters, m.header.FunctionCode, err)
		}
		bytes = append(bytes, protocol.InjectModbusHeader(m.header, protocol.CoilsToBytes(result))...)
	case protocol.ReadHoldingRegisters:
		result, err := m.realClient.ReadRegisters(m.header.Address, m.header.NumberOfCoilsOrRegisters, modbus.HOLDING_REGISTER)
		if err != nil {
			return 0, fmt.Errorf("error while reading holding register %v, with length %v and function code %v, the error that occurred was %v", m.header.Address, m.header.NumberOfCoilsOrRegisters, m.header.FunctionCode, err)
		}
		bytes = append(bytes, protocol.InjectModbusHeader(m.header, protocol.RegistersToBytes(result))...)
	case protocol.ReadInputRegisters:
		result, err := m.realClient.ReadRegisters(m.header.Address, m.header.NumberOfCoilsOrRegisters, modbus.INPUT_REGISTER)
		if err != nil {
			return 0, fmt.Errorf("error while reading input register %v, with length %v and function code %v, the error that occurred was %v", m.header.Address, m.header.NumberOfCoilsOrRegisters, m.header.FunctionCode, err)
		}
		bytes = append(bytes, protocol.InjectModbusHeader(m.header, protocol.RegistersToBytes(result))...)
	default:
		return 0, fmt.Errorf("unsupported function code type %v", m.header.FunctionCode)
	}
	return
}

func (m *ModbusClient) Write(bytes []byte) (n int, err error) {
	header, bytes, err := protocol.ExtractModbusHeader(bytes)
	if err != nil {
		return 0, err
	}

	m.realClient.SetUnitId(header.Slave)
	switch header.FunctionCode {
	case protocol.WriteSingleCoil:
		if header.NumberOfCoilsOrRegisters != 1 {
			return 0, fmt.Errorf("expected only 1 register but got %d", header.NumberOfCoilsOrRegisters)
		}
		coils, err := protocol.BytesToCoils(bytes, int(header.NumberOfCoilsOrRegisters))
		if err != nil {
			return 0, err
		}
		m.realClient.WriteCoil(header.Address, coils[0])
	case protocol.WriteSingleRegister:
		if header.NumberOfCoilsOrRegisters != 1 {
			return 0, fmt.Errorf("expected only 1 register but got %d", header.NumberOfCoilsOrRegisters)
		}
		registers, err := protocol.BytesToRegisters(bytes, int(header.NumberOfCoilsOrRegisters))
		if err != nil {
			return 0, err
		}
		m.realClient.WriteRegister(header.Address, registers[0])
	case protocol.WriteMultipleCoils:
		coils, err := protocol.BytesToCoils(bytes, int(header.NumberOfCoilsOrRegisters))
		if err != nil {
			return 0, err
		}
		m.realClient.WriteCoils(header.Address, coils)
	case protocol.WriteMultipleRegisters:
		registers, err := protocol.BytesToRegisters(bytes, int(header.NumberOfCoilsOrRegisters))
		if err != nil {
			return 0, err
		}
		m.realClient.WriteRegisters(header.Address, registers)
	default:
		return 0, fmt.Errorf("unsupported function code type %v", header.FunctionCode)
	}
	return len(bytes), nil
}

func (m *ModbusClient) Poll(stream chan<- []byte, pollingInterval time.Duration) error {
	ticker := time.NewTicker(pollingInterval)
	done := make(chan struct{})
	var bytes []byte
	if m.header.FunctionCode == protocol.ReadCoils || m.header.FunctionCode == protocol.ReadDiscreteInputs {
		bytes = make([]byte, 0, (m.header.NumberOfCoilsOrRegisters-1)/8+1)
	} else if m.header.FunctionCode == protocol.ReadHoldingRegisters || m.header.FunctionCode == protocol.ReadInputRegisters {
		bytes = make([]byte, 0, m.header.NumberOfCoilsOrRegisters*2)
	}
	for {
		select {
		case <-ticker.C:
			n, err := m.Read(bytes)
			// TODO: how to handle failed reads, never attempt again or keep trying
			if err != nil {
				return err
			}
			stream <- bytes[:n]
		case <-done:
			ticker.Stop()
			return nil
		}
	}
}
