package protocol

import (
	"encoding/binary"
	"fmt"
)

const (
	HeaderLength = 7

	// MaximumNumberOfRegisters is maximum number of registers that can be read in one request, a modbus message is limit to 256 bytes
	// TODO: this should be checked when register groups are created
	MaximumNumberOfRegisters = 125
	MaximumNumberOfCoils     = 2000
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
	ReportServerId = 0x11
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

func CoilsToBytes(values []bool) []byte {
	bytes := make([]byte, (len(values)-1)/8+1)
	for i, v := range values {
		if v {
			bytes[i/8] += 1 << (7 - i%8)
		}
	}
	return bytes
}

func BytesToCoils(bytes []byte, numberOfCoils int) ([]bool, error) {
	if len(bytes) != (numberOfCoils-1)/8+1 {
		return nil, fmt.Errorf("expected %d bytes, got %d bytes", numberOfCoils/8+1, len(bytes))
	}

	coils := make([]bool, 0, numberOfCoils)
	for i := 0; i < numberOfCoils; i++ {
		coils[i] = (bytes[i/8] & 1 << (7 - i%8)) == 1<<(7-i%8)
	}

	return coils, nil
}

func RegistersToBytes(values []uint16) []byte {
	bytes := make([]byte, 0, 2*len(values))
	out := make([]byte, 2)
	for _, v := range values {
		// TODO: make BigEndian / LittleEndian configurable
		binary.BigEndian.PutUint16(out, v)
		bytes = append(bytes, out...)
	}
	return bytes
}

func BytesToRegisters(bytes []byte, numberOfRegisters int) ([]uint16, error) {
	if len(bytes) != numberOfRegisters*2 {
		return nil, fmt.Errorf("expected %d bytes, got %d bytes", numberOfRegisters*2, len(bytes))
	}
	registers := make([]uint16, 0, numberOfRegisters)
	for i := 0; i < numberOfRegisters; i++ {
		registers = append(registers, binary.BigEndian.Uint16(bytes[i*2:i*2+2]))
	}

	return registers, nil
}

func InjectModbusHeader(header *ModbusHeader, bytes []byte) []byte {
	headerBytes := make([]byte, 0, HeaderLength)
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
	if len(bytes) < HeaderLength {
		return nil, nil, fmt.Errorf("expected at least %d bytes but got %d", HeaderLength, len(bytes))
	}

	header := &ModbusHeader{
		Slave:                    uint8(bytes[0]),
		FunctionCode:             binary.BigEndian.Uint16(bytes[1:3]),
		Address:                  binary.BigEndian.Uint16(bytes[3:5]),
		NumberOfCoilsOrRegisters: binary.BigEndian.Uint16(bytes[5:7]),
	}

	return header, bytes[HeaderLength:], nil
}
