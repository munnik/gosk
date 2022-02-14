package collector

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/simonvetter/modbus"
	"go.nanomsg.org/mangos/v3"
)

// MaximumNumberOfRegisters is maximum number of registers that can be read in one request, a modbus message is limit to 256 bytes
// TODO: this should be checked when register groups are created
const MaximumNumberOfRegisters = 120
const (
	Coils = iota + 1
	DiscreteInputs
	HoldingRegisters
	InputRegisters
)

type ModbusReader struct {
	config               config.CollectorConfig
	registerGroupsConfig []config.RegisterGroupConfig
}

func NewModbusReader(c config.CollectorConfig, rgs []config.RegisterGroupConfig) (*ModbusReader, error) {
	for _, rg := range rgs {
		if rg.NumberOfRegisters > MaximumNumberOfRegisters {
			return nil, fmt.Errorf("maximum number %v of registers exceeded for register group %v", MaximumNumberOfRegisters, rg)
		}
	}
	return &ModbusReader{config: c, registerGroupsConfig: rgs}, nil
}

func (m *ModbusReader) Collect(publisher mangos.Socket) {
	stream := make(chan []byte, 1)
	go m.receive(stream)
	process(stream, m.config.Name, publisher)
}

func (m *ModbusReader) receive(stream chan<- []byte) error {
	defer close(stream)

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
		URL:      m.config.URI.String(),
		Speed:    uint(m.config.BaudRate),
		DataBits: uint(m.config.DataBits),
		Parity:   uint(m.config.Parity),
		StopBits: uint(m.config.StopBits),
		Timeout:  1 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create modbus client %v, the error that occurred was %v", m.config.URI.String(), err)
	}
	if err := client.Open(); err != nil {
		return nil, fmt.Errorf("unable to open modbus client %v, the error that occurred was %v", m.config.URI.String(), err)
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

func (m *ModbusClient) Read(slave uint8, functionCode uint16, address uint16, numberOfRegisters uint16) ([]byte, error) {
	var bytes []byte

	m.mu.Lock()
	defer m.mu.Unlock()
	m.realClient.SetUnitId(slave)
	switch functionCode {
	case Coils:
		result, err := m.realClient.ReadCoils(address, numberOfRegisters)
		if err != nil {
			return nil, fmt.Errorf("error while reading register %v, with length %v and function code %v, , the error that occurred was %v", address, numberOfRegisters, functionCode, err)
		}
		bytes = boolsToBytes(slave, functionCode, address, numberOfRegisters, result)
	case DiscreteInputs:
		result, err := m.realClient.ReadDiscreteInputs(address, numberOfRegisters)
		if err != nil {
			return nil, fmt.Errorf("error while reading register %v, with length %v and function code %v, , the error that occurred was %v", address, numberOfRegisters, functionCode, err)
		}
		bytes = boolsToBytes(slave, functionCode, address, numberOfRegisters, result)
	case HoldingRegisters:
		result, err := m.realClient.ReadRegisters(address, numberOfRegisters, modbus.HOLDING_REGISTER)
		if err != nil {
			return nil, fmt.Errorf("error while reading register %v, with length %v and function code %v, , the error that occurred was %v", address, numberOfRegisters, functionCode, err)
		}
		bytes = unint16sToBytes(slave, functionCode, address, numberOfRegisters, result)
	case InputRegisters:
		result, err := m.realClient.ReadRegisters(address, numberOfRegisters, modbus.INPUT_REGISTER)
		if err != nil {
			return nil, fmt.Errorf("error while reading register %v, with length %v and function code %v, , the error that occurred was %v", address, numberOfRegisters, functionCode, err)
		}
		bytes = unint16sToBytes(slave, functionCode, address, numberOfRegisters, result)
	default:
		return nil, fmt.Errorf("unsupported function code type %v", functionCode)
	}
	return bytes, nil
}

func (m *ModbusClient) Poll(stream chan<- []byte, rgc config.RegisterGroupConfig) error {
	ticker := time.NewTicker(rgc.PollingInterval)
	done := make(chan struct{})
	for {
		select {
		case <-ticker.C:
			bytes, err := m.Read(rgc.Slave, rgc.FunctionCode, rgc.Address, rgc.NumberOfRegisters)
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

func boolsToBytes(slave uint8, functionCode uint16, address uint16, numberOfRegisters uint16, values []bool) []byte {
	uint16s := make([]uint16, (len(values)-1)/16+1)
	for i, v := range values {
		if v {
			// TODO: make BigEndian / LittleEndian configurable
			uint16s[i/16] += 1 << (15 - i%16)
		}
	}
	return unint16sToBytes(slave, functionCode, address, numberOfRegisters, uint16s)
}

func unint16sToBytes(slave uint8, functionCode uint16, address uint16, numberOfRegisters uint16, values []uint16) []byte {
	bytes := make([]byte, 0, 7+2*len(values)) // 7 is the length of the start bytes
	bytes = append(bytes, startBytes(slave, functionCode, address, numberOfRegisters)...)
	out := make([]byte, 2)
	for _, v := range values {
		// TODO: make BigEndian / LittleEndian configurable
		binary.BigEndian.PutUint16(out, v)
		bytes = append(bytes, out...)
	}
	return bytes
}

func startBytes(slave uint8, functionCode uint16, address uint16, numberOfRegisters uint16) []byte {
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
