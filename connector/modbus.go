package connector

import (
	"fmt"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/protocol"
	"github.com/munnik/modbus"
	"go.bug.st/serial"
	"go.uber.org/zap"
)

type ModbusConnector struct {
	config               *config.ConnectorConfig
	registerGroupsConfig []config.RegisterGroupConfig
	realClient           *modbus.Client
}

func NewModbusConnector(c *config.ConnectorConfig, rgcs []config.RegisterGroupConfig) (*ModbusConnector, error) {
	for _, rgc := range rgcs {
		// TODO add write function codes
		if rgc.FunctionCode == protocol.ReadCoils || rgc.FunctionCode == protocol.ReadDiscreteInputs {
			if rgc.NumberOfCoilsOrRegisters > protocol.MODBUS_MAXIMUM_NUMBER_OF_COILS {
				return nil, fmt.Errorf("maximum number %v of coils exceeded for register group %v", protocol.MODBUS_MAXIMUM_NUMBER_OF_COILS, rgc)
			}
		} else {
			if rgc.NumberOfCoilsOrRegisters > protocol.MODBUS_MAXIMUM_NUMBER_OF_REGISTERS {
				return nil, fmt.Errorf("maximum number %v of registers exceeded for register group %v", protocol.MODBUS_MAXIMUM_NUMBER_OF_REGISTERS, rgc)
			}
		}
	}
	cc := &modbus.Configuration{
		URL:      c.URL.String(),
		Speed:    c.BaudRate,
		DataBits: c.DataBits,
		Timeout:  1 * time.Second,
	}
	switch c.StopBits {
	case "1":
		cc.StopBits = serial.OneStopBit
	case "1.5":
		cc.StopBits = serial.OnePointFiveStopBits
	case "2":
		cc.StopBits = serial.TwoStopBits
	default:
		return nil, fmt.Errorf("unsupport stop bits: %s", c.StopBits)
	}
	switch c.Parity {
	case "N":
		cc.Parity = serial.NoParity
	case "O":
		cc.Parity = serial.OddParity
	case "E":
		cc.Parity = serial.EvenParity
	default:
		return nil, fmt.Errorf("unsupport parity: %s", c.Parity)
	}
	realClient, err := modbus.NewClient(cc)
	if err != nil {
		return nil, fmt.Errorf("unable to create modbus client %v, the error that occurred was %v", c.URL.String(), err)
	}

	return &ModbusConnector{config: c, registerGroupsConfig: rgcs, realClient: realClient}, nil
}

func (m *ModbusConnector) Publish(publisher *nanomsg.Publisher[message.Raw]) {
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

func (m *ModbusConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	go func() {
		client := protocol.NewModbusClient(
			m.realClient,
			nil, // no need to set this because it will not be used in the Write([]byte) function
		)
		receiveBuffer := make(chan *message.Raw, bufferCapacity)
		defer close(receiveBuffer)
		go subscriber.Receive(receiveBuffer)

		for raw := range receiveBuffer {
			if _, err := client.Write(raw.Value); err != nil {
				logger.GetLogger().Warn(
					"Error while writing data",
					zap.String("URL", m.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
}

func (m *ModbusConnector) receive(stream chan<- []byte) error {
	errors := make(chan error)
	done := make(chan bool)
	var wg sync.WaitGroup
	wg.Add(len(m.registerGroupsConfig))

	// start a go routine for each register group, if an error occurs send it on the error channel
	for _, rgc := range m.registerGroupsConfig {
		go func(rgc config.RegisterGroupConfig) {
			client := protocol.NewModbusClient(
				m.realClient,
				rgc.ExtractModbusHeader(),
			)
			fmt.Printf("Created a new modbus cient for slave %d, function code %d, address %d, # registers %d\n", rgc.Slave, rgc.FunctionCode, rgc.Address, rgc.NumberOfCoilsOrRegisters)
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
