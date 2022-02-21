package collector

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/simonvetter/modbus"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// MaximumNumberOfRegisters is maximum number of registers that can be read in one request, a modbus message is limit to 256 bytes
// TODO: this should be checked when register groups are created
const MaximumNumberOfRegisters = 125
const MaximumNumberOfCoils = 2000

const (
	Coils = iota + 1
	DiscreteInputs
	HoldingRegisters
	InputRegisters
)

type ModbusReader struct {
	config               *config.CollectorConfig
	registerGroupsConfig []config.RegisterGroupConfig
}

func NewModbusReader(c *config.CollectorConfig, rgs []config.RegisterGroupConfig) (*ModbusReader, error) {
	for _, rg := range rgs {
		if rg.NumberOfRegisters > MaximumNumberOfRegisters {
			return nil, fmt.Errorf("maximum number %v of registers exceeded for register group %v", MaximumNumberOfRegisters, rg)
		}
		if rg.NumberOfCoils > MaximumNumberOfCoils {
			return nil, fmt.Errorf("maximum number %v of coils exceeded for register group %v", MaximumNumberOfCoils, rg)
		}
	}
	return &ModbusReader{config: c, registerGroupsConfig: rgs}, nil
}

func (r *ModbusReader) Collect(publisher mangos.Socket) {
	stream := make(chan []byte, 1)
	defer close(stream)
	go func() {
		for {
			if err := r.receive(stream); err != nil {
				logger.GetLogger().Warn(
					"Error while receiving data for the stream",
					zap.String("URL", r.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
	process(stream, r.config.Name, r.config.Protocol, publisher)
}

func (m *ModbusReader) receive(stream chan<- []byte) error {
	client, err := m.createClient()
	if err != nil {
		return err
	}
	defer client.close()

	errors := make(chan error)
	done := make(chan bool)
	var wg sync.WaitGroup
	wg.Add(len(m.registerGroupsConfig))

	// start a go routine for each register group, if an error occurs send it on the error channel
	for _, rgc := range m.registerGroupsConfig {
		go func(client *ModbusClient, rgc config.RegisterGroupConfig) {
			if err := client.Poll(stream, rgc); err != nil {
				errors <- err
			}
			wg.Done()
		}(client, rgc)
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

func (m ModbusReader) createClient() (*ModbusClient, error) {
	client, err := modbus.NewClient(&modbus.ClientConfiguration{
		URL:      m.config.URL.String(),
		Speed:    uint(m.config.BaudRate),
		DataBits: uint(m.config.DataBits),
		Parity:   uint(m.config.Parity),
		StopBits: uint(m.config.StopBits),
		Timeout:  1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create modbus client %v, the error that occurred was %v", m.config.URL.String(), err)
	}
	if err := client.Open(); err != nil {
		return nil, fmt.Errorf("unable to open modbus client %v, the error that occurred was %v", m.config.URL.String(), err)
	}
	return &ModbusClient{realClient: client}, nil
}

type ModbusClient struct {
	mu         sync.Mutex // used to make sure that only one read at a time can occur
	realClient *modbus.ModbusClient
}

func (m *ModbusClient) close() {
	m.mu.Lock()
	m.realClient.Close()
	m.mu.Unlock()
}

func (m *ModbusClient) Read(rgc config.RegisterGroupConfig) ([]byte, error) {
	var bytes []byte

	m.mu.Lock()
	defer m.mu.Unlock()
	m.realClient.SetUnitId(rgc.Slave)
	switch rgc.FunctionCode {
	case Coils:
		result, err := m.realClient.ReadCoils(rgc.Address, rgc.NumberOfCoils)
		if err != nil {
			return nil, fmt.Errorf("error while reading register %v, with length %v and function code %v, , the error that occurred was %v", rgc.Address, rgc.NumberOfCoils, rgc.FunctionCode, err)
		}
		bytes = CoilsToBytes(rgc, result)
	case DiscreteInputs:
		result, err := m.realClient.ReadDiscreteInputs(rgc.Address, rgc.NumberOfCoils)
		if err != nil {
			return nil, fmt.Errorf("error while reading register %v, with length %v and function code %v, , the error that occurred was %v", rgc.Address, rgc.NumberOfCoils, rgc.FunctionCode, err)
		}
		bytes = CoilsToBytes(rgc, result)
	case HoldingRegisters:
		result, err := m.realClient.ReadRegisters(rgc.Address, rgc.NumberOfRegisters, modbus.HOLDING_REGISTER)
		if err != nil {
			return nil, fmt.Errorf("error while reading register %v, with length %v and function code %v, , the error that occurred was %v", rgc.Address, rgc.NumberOfRegisters, rgc.FunctionCode, err)
		}
		bytes = RegistersToBytes(rgc, result)
	case InputRegisters:
		result, err := m.realClient.ReadRegisters(rgc.Address, rgc.NumberOfRegisters, modbus.INPUT_REGISTER)
		if err != nil {
			return nil, fmt.Errorf("error while reading register %v, with length %v and function code %v, , the error that occurred was %v", rgc.Address, rgc.NumberOfRegisters, rgc.FunctionCode, err)
		}
		bytes = RegistersToBytes(rgc, result)
	default:
		return nil, fmt.Errorf("unsupported function code type %v", rgc.FunctionCode)
	}
	return bytes, nil
}

func (m *ModbusClient) Poll(stream chan<- []byte, rgc config.RegisterGroupConfig) error {
	ticker := time.NewTicker(rgc.PollingInterval)
	done := make(chan struct{})
	for {
		select {
		case <-ticker.C:
			bytes, err := m.Read(rgc)
			// TODO: how to handle failed reads, never attempt again or keep trying
			if err != nil {
				return err
			}
			stream <- bytes
		case <-done:
			ticker.Stop()
			return nil
		}
	}
}

func CoilsToBytes(rgc config.RegisterGroupConfig, values []bool) []byte {
	uint16s := make([]uint16, (len(values)-1)/16+1)
	for i, v := range values {
		if v {
			// TODO: make BigEndian / LittleEndian configurable
			uint16s[i/16] += 1 << (15 - i%16)
		}
	}
	rgc.NumberOfRegisters = (rgc.NumberOfCoils-1)/16 + 1
	return RegistersToBytes(rgc, uint16s)
}

func RegistersToBytes(rgc config.RegisterGroupConfig, values []uint16) []byte {
	bytes := make([]byte, 0, 7+2*len(values)) // 7 is the length of the start bytes
	bytes = append(bytes, StartBytes(rgc.Slave, rgc.FunctionCode, rgc.Address, rgc.NumberOfRegisters)...)
	out := make([]byte, 2)
	for _, v := range values {
		// TODO: make BigEndian / LittleEndian configurable
		binary.BigEndian.PutUint16(out, v)
		bytes = append(bytes, out...)
	}
	return bytes
}

func StartBytes(slave uint8, functionCode uint16, address uint16, numberOfRegisters uint16) []byte {
	bytes := []byte{byte(slave)}
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, functionCode)
	bytes = append(bytes, out...)
	binary.BigEndian.PutUint16(out, address)
	bytes = append(bytes, out...)
	binary.BigEndian.PutUint16(out, numberOfRegisters)
	bytes = append(bytes, out...)
	return bytes
}
