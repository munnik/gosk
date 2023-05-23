package protocol

import (
	"encoding/binary"
	"fmt"
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
	READ_COILS = 0x01
	// 02 (0x02) Read Discrete Inputs
	READ_DISCRETE_INPUTS = 0x02
	// 03 (0x03) Read Holding Registers
	READ_HOLDING_REGISTERS = 0x03
	// 04 (0x04) Read Input Registers
	READ_INPUT_REGISTERS = 0x04
	// 05 (0x05) Write Single Coil
	WRITE_SINGLE_COIL = 0x05
	// 06 (0x06) Write Single Register
	WRITE_SINGLE_REGISTER = 0x06
	// 08 (0x08) Diagnostics (Serial Line only)
	DIAGNOSTICS = 0x08
	// 11 (0x0B) Get Comm Event Counter (Serial Line only)
	GET_COMM_EVENT_COUNTER = 0x0B
	// 15 (0x0F) Write Multiple Coils
	WRITE_MULTIPLE_COILS = 0x0F
	// 16 (0x10) Write Multiple Registers
	WRITE_MULTIPLE_REGISTERS = 0x10
	// 17 (0x11) Report Server ID (Serial Line only)
	REPORT_SERVER_ID = 0x11
	// 22 (0x16) Mask Write Register
	MASK_WRITE_REGISTERS = 0x16
	// 23 (0x17) Read/Write Multiple Registers
	READ_WRITE_MULTIPLE_REGISTERS = 0x17
	// 43 / 14 (0x2B / 0x0E) Read Device Identification
	READ_DEVICE_IDENTIFICATION_A = 0x0E
	READ_DEVICE_IDENTIFICATION_B = 0x43
)

type ModbusHeader struct {
	Slave                    uint8  `mapstructure:"slave"`
	FunctionCode             uint16 `mapstructure:"functionCode"`
	Address                  uint16 `mapstructure:"address"`
	NumberOfCoilsOrRegisters uint16 `mapstructure:"numberOfCoilsOrRegisters"`
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
		return nil, nil, fmt.Errorf("expected at least %d bytes but got %d", MODBUS_HEADER_LENGTH, len(bytes))
	}

	header := &ModbusHeader{
		Slave:                    uint8(bytes[0]),
		FunctionCode:             binary.BigEndian.Uint16(bytes[1:3]),
		Address:                  binary.BigEndian.Uint16(bytes[3:5]),
		NumberOfCoilsOrRegisters: binary.BigEndian.Uint16(bytes[5:7]),
	}

	return header, bytes[MODBUS_HEADER_LENGTH:], nil
}
