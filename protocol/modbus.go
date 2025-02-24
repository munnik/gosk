package protocol

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/modbus"
	"go.uber.org/zap"
)

const (
	MODBUS_HEADER_LENGTH = 7

	// MODBUS_Maximum_Number_Of_Registers is maximum number of registers that can be read in one request, a modbus message is limit to 256 bytes
	// TODO: this should be checked when register groups are created
	MODBUS_MAXIMUM_NUMBER_OF_REGISTERS = 125
	MODBUS_MAXIMUM_NUMBER_OF_COILS     = 2000
)

const (
	// 01 (0x01) Read Coils
	ReadCoils = 0x01
	// 02 (0x02) Read Discrete Inputs
	ReadDiscreteInputs = 0x02
	// 03 (0x03) Read Holding Registers
	ReadHoldingRegisters = 0x03
	// 04 (0x04) Read Input Registers
	ReadInputRegisters = 0x04
	// 05 (0x05) Write Single Coil
	WriteSingleCoil = 0x05
	// 06 (0x06) Write Single Register
	WriteSingleRegister = 0x06
	// 08 (0x08) Diagnostics (Serial Line only)
	Diagnostics = 0x08
	// 11 (0x0B) Get Comm Event Counter (Serial Line only)
	GetCommEventCounter = 0x0B
	// 15 (0x0F) Write Multiple Coils
	WriteMultipleCoils = 0x0F
	// 16 (0x10) Write Multiple Registers
	WriteMultipleRegisters = 0x10
	// 17 (0x11) Report Server ID (Serial Line only)
	ReportServerID = 0x11
	// 22 (0x16) Mask Write Register
	MaskWriteRegisters = 0x16
	// 23 (0x17) Read/Write Multiple Registers
	ReadWriteMultipleRegisters = 0x17
	// 43 / 14 (0x2B / 0x0E) Read Device Identification
	ReadDeviceIdentificationA = 0x0E
	ReadDeviceIdentificationB = 0x43
)

type ModbusHeader struct {
	Slave                    uint8  `mapstructure:"slave"`
	FunctionCode             uint16 `mapstructure:"functionCode"`
	Address                  uint16 `mapstructure:"address"`
	NumberOfCoilsOrRegisters uint16 `mapstructure:"numberOfCoilsOrRegisters"`
}

type ModbusClient struct {
	realClient *modbus.Client
	header     *ModbusHeader
}

func NewModbusClient(realClient *modbus.Client, header *ModbusHeader) *ModbusClient {
	return &ModbusClient{
		realClient: realClient,
		header:     header,
	}
}

func (m *ModbusClient) Read(bytes []byte) (int, error) {
	return m.execute(m.header, bytes)
}

func (m *ModbusClient) Write(bytes []byte) (int, error) {
	header, bytes, err := ExtractModbusHeader(bytes)
	if err != nil {
		return 0, err
	}
	count, err := m.execute(header, bytes)
	if err != nil {
		m.realClient.Close()
		return 0, err
	}
	return count, nil
}

func (m *ModbusClient) execute(header *ModbusHeader, bytes []byte) (int, error) {
	if err := m.realClient.Open(); err != nil && err != modbus.ErrTransportIsAlreadyOpen {
		logger.GetLogger().Error(
			"Could not open real client",
			zap.Error(err),
		)
		return 0, err
	}

	switch header.FunctionCode {
	case ReadCoils:
		result, err := m.realClient.ReadCoils(m.header.Address, m.header.NumberOfCoilsOrRegisters, modbus.WithUnitID(m.header.Slave))
		if err != nil {
			return 0, fmt.Errorf("error while reading slave %v coils %v, with length %v and function code %v, the error that occurred was %v", m.header.Slave, m.header.Address, m.header.NumberOfCoilsOrRegisters, m.header.FunctionCode, err)
		}
		bytes = bytes[:0]
		bytes = append(bytes, InjectModbusHeader(m.header, CoilsToBytes(result))...)
	case ReadDiscreteInputs:
		result, err := m.realClient.ReadDiscreteInputs(m.header.Address, m.header.NumberOfCoilsOrRegisters, modbus.WithUnitID(m.header.Slave))
		if err != nil {
			return 0, fmt.Errorf("error while reading slave %v discrete inputs %v, with length %v and function code %v, the error that occurred was %v", m.header.Slave, m.header.Address, m.header.NumberOfCoilsOrRegisters, m.header.FunctionCode, err)
		}
		bytes = bytes[:0]
		bytes = append(bytes, InjectModbusHeader(m.header, CoilsToBytes(result))...)
	case ReadHoldingRegisters:
		result, err := m.realClient.ReadRegisters(m.header.Address, m.header.NumberOfCoilsOrRegisters, modbus.HoldingRegister, modbus.WithUnitID(m.header.Slave))
		if err != nil {
			return 0, fmt.Errorf("error while reading slave %v holding register %v, with length %v and function code %v, the error that occurred was %v", m.header.Slave, m.header.Address, m.header.NumberOfCoilsOrRegisters, m.header.FunctionCode, err)
		}
		bytes = bytes[:0]
		bytes = append(bytes, InjectModbusHeader(m.header, RegistersToBytes(result))...)
	case ReadInputRegisters:
		result, err := m.realClient.ReadRegisters(m.header.Address, m.header.NumberOfCoilsOrRegisters, modbus.InputRegister, modbus.WithUnitID(m.header.Slave))
		if err != nil {
			return 0, fmt.Errorf("error while reading slave %v input register %v, with length %v and function code %v, the error that occurred was %v", m.header.Slave, m.header.Address, m.header.NumberOfCoilsOrRegisters, m.header.FunctionCode, err)
		}
		bytes = bytes[:0]
		bytes = append(bytes, InjectModbusHeader(m.header, RegistersToBytes(result))...)
	case WriteSingleCoil:
		if header.NumberOfCoilsOrRegisters != 1 {
			return 0, fmt.Errorf("expected only 1 register but got %d", header.NumberOfCoilsOrRegisters)
		}
		coils, err := BytesToCoils(bytes)
		if err != nil {
			return 0, err
		}
		m.realClient.WriteCoil(header.Address, coils[0], modbus.WithUnitID(header.Slave))
	case WriteSingleRegister:
		if header.NumberOfCoilsOrRegisters != 1 {
			return 0, fmt.Errorf("expected only 1 register but got %d", header.NumberOfCoilsOrRegisters)
		}
		registers, err := BytesToRegisters(bytes)
		if err != nil {
			return 0, err
		}
		if len(registers) != int(header.NumberOfCoilsOrRegisters) {
			return 0, fmt.Errorf("expected %d registers but got %d register", header.NumberOfCoilsOrRegisters, len(registers))
		}
		m.realClient.WriteRegister(header.Address, registers[0], modbus.WithUnitID(header.Slave))
	case WriteMultipleCoils:
		coils, err := BytesToCoils(bytes)
		if err != nil {
			return 0, err
		}
		m.realClient.WriteCoils(header.Address, coils, modbus.WithUnitID(header.Slave))
	case WriteMultipleRegisters:
		registers, err := BytesToRegisters(bytes)
		if err != nil {
			return 0, err
		}
		if len(registers) != int(header.NumberOfCoilsOrRegisters) {
			return 0, fmt.Errorf("expected %d registers but got %d register", header.NumberOfCoilsOrRegisters, len(registers))
		}
		m.realClient.WriteRegisters(header.Address, registers, modbus.WithUnitID(header.Slave))
	default:
		return 0, fmt.Errorf("unsupported function code type %v", header.FunctionCode)
	}
	return len(bytes), nil
}

func (m *ModbusClient) Poll(stream chan<- []byte, pollingInterval time.Duration) error {
	ticker := time.NewTicker(pollingInterval)
	done := make(chan struct{})
	bytes := make([]byte, 0, m.header.NumberOfCoilsOrRegisters*2+MODBUS_HEADER_LENGTH)
	for {
		select {
		case <-ticker.C:
			n, err := m.Read(bytes)
			// TODO: how to handle failed reads, never attempt again or keep trying
			if err != nil {
				logger.GetLogger().Warn(
					"Error while reading",
					zap.Error(err),
				)
				continue
			}

			stream <- bytes[:n]
		case <-done:
			ticker.Stop()
			return nil
		}
	}
}

func CoilsToBytes(values []bool) []byte {
	bytes := make([]byte, len(values)*2)
	for i, v := range values {
		if v {
			bytes[i*2] = 0xff
			bytes[i*2+1] = 0x00
		}
	}
	return bytes
}

func BytesToCoils(bytes []byte) ([]bool, error) {
	registers, err := BytesToRegisters(bytes)
	if err != nil {
		return nil, err
	}
	return RegistersToCoils(registers), nil
}

func RegistersToBytes(values []uint16) []byte {
	bytes := make([]byte, 0, 2*len(values))
	out := make([]byte, 2)
	for _, v := range values {
		binary.BigEndian.PutUint16(out, v)
		bytes = append(bytes, out...)
	}
	return bytes
}

func BytesToRegisters(bytes []byte) ([]uint16, error) {
	if len(bytes)%2 != 0 {
		return nil, fmt.Errorf("expected even number of bytes, got %d bytes", len(bytes))
	}
	numberOfRegisters := len(bytes) / 2
	registers := make([]uint16, 0, numberOfRegisters)
	for i := 0; i < numberOfRegisters; i++ {
		registers = append(registers, binary.BigEndian.Uint16(bytes[i*2:i*2+2]))
	}

	return registers, nil
}

func RegistersToCoils(registers []uint16) []bool {
	coils := make([]bool, len(registers))
	for i := range registers {
		if registers[i] == 0xff00 {
			coils[i] = true
		}
	}
	return coils
}

func CoilsToRegisters(coils []bool) []uint16 {
	registers := make([]uint16, len(coils))
	for i := range coils {
		if coils[i] {
			registers[i] = 0xff00
		}
	}
	return registers
}

func InjectModbusHeader(header *ModbusHeader, bytes []byte) []byte {
	headerBytes := make([]byte, 0, MODBUS_HEADER_LENGTH)
	headerBytes = append(headerBytes, byte(header.Slave))
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, header.FunctionCode)
	headerBytes = append(headerBytes, out...)
	binary.BigEndian.PutUint16(out, header.Address)
	headerBytes = append(headerBytes, out...)
	binary.BigEndian.PutUint16(out, header.NumberOfCoilsOrRegisters)
	headerBytes = append(headerBytes, out...)

	return append(headerBytes, bytes...)
}

func ExtractModbusHeader(bytes []byte) (*ModbusHeader, []byte, error) {
	if len(bytes) < MODBUS_HEADER_LENGTH {
		return nil, nil, fmt.Errorf("unable to extract the modbus header, expected at least %d bytes but got %d", MODBUS_HEADER_LENGTH, len(bytes))
	}

	header := &ModbusHeader{
		Slave:                    uint8(bytes[0]),
		FunctionCode:             binary.BigEndian.Uint16(bytes[1:3]),
		Address:                  binary.BigEndian.Uint16(bytes[3:5]),
		NumberOfCoilsOrRegisters: binary.BigEndian.Uint16(bytes[5:7]),
	}

	return header, bytes[MODBUS_HEADER_LENGTH:], nil
}
