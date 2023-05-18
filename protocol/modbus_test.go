package protocol_test

import (
	"github.com/munnik/gosk/protocol"
	. "github.com/munnik/gosk/protocol"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Modbus protocol functions", func() {
	DescribeTable(
		"RegistersToCoils",
		func(input []uint16, numberOfCoils uint16, expected []bool) {
			result := protocol.RegistersToCoils(input, numberOfCoils)
			Expect(result).To(Equal(expected))
		},
		Entry("One register with value 0x0000 and number of coils 2", []uint16{0x0000}, uint16(2), []bool{
			false,
			false,
		}),
		Entry("One register with value 0x0001 and number of coils 3", []uint16{0x8002}, uint16(3), []bool{
			true,
			false,
			false,
		}),
		Entry("Three registers with value 0x45a3 0x7812 0x0001 and number of coils 47", []uint16{0x45a3, 0x7812, 0x0001}, uint16(47), []bool{
			false,
			true,
			false,
			false,
			false,
			true,
			false,
			true,
			true,
			false,
			true,
			false,
			false,
			false,
			true,
			true,

			false,
			true,
			true,
			true,
			true,
			false,
			false,
			false,
			false,
			false,
			false,
			true,
			false,
			false,
			true,
			false,

			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
		}),
	)
	DescribeTable(
		"CoilsToRegisters",
		func(expected []uint16, input []bool) {
			result := protocol.CoilsToRegisters(input)
			Expect(result).To(Equal(expected))
		},
		Entry("2 coils, both false", []uint16{0x0000}, []bool{
			false,
			false,
		}),
		Entry("3 coils, first true rest false", []uint16{0x8000}, []bool{
			true,
			false,
			false,
		}),
		Entry("47 coils", []uint16{0x45a3, 0x7812, 0x0000}, []bool{
			false,
			true,
			false,
			false,
			false,
			true,
			false,
			true,
			true,
			false,
			true,
			false,
			false,
			false,
			true,
			true,

			false,
			true,
			true,
			true,
			true,
			false,
			false,
			false,
			false,
			false,
			false,
			true,
			false,
			false,
			true,
			false,

			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
			false,
		}),
	)
	Describe("CoilsToBytes", func() {
		var (
			header *ModbusHeader

			input  []bool
			result []byte
		)
		JustBeforeEach(func() {
			result = InjectModbusHeader(header, CoilsToBytes(input))
		})
		Context("with 8 coils", func() {
			BeforeEach(func() {
				header = &ModbusHeader{
					Slave:                    1,
					FunctionCode:             2,
					Address:                  50,
					NumberOfCoilsOrRegisters: 8,
				}
				// 128 + 64 + _ + _ 8 + _ + 2 + 1 = 203
				input = []bool{true, true, false, false, true, false, true, true}
			})
			It("equals the correct bytes", func() {
				Expect(result).To(Equal([]byte{
					1,  // slave
					0,  // msb function code
					2,  // lsb function code
					0,  // msb address
					50, // lsb address
					0,  // msb number of coils or registers
					8,  // lsb number of coils or registers
					203,
				}))
			})
		})
		Context("with 5 coils", func() {
			BeforeEach(func() {
				header = &ModbusHeader{
					Slave:                    1,
					FunctionCode:             2,
					Address:                  50,
					NumberOfCoilsOrRegisters: 5,
				}
				// 128 + 64 + _ + _ 8 = 200
				input = []bool{true, true, false, false, true}
			})
			It("equals the correct bytes", func() {
				Expect(result).To(Equal([]byte{
					1,  // slave
					0,  // msb function code
					2,  // lsb function code
					0,  // msb address
					50, // lsb address
					0,  // msb number of coils or registers
					5,  // lsb number of coils or registers
					200,
				}))
			})
		})
		Context("with 16 coils", func() {
			BeforeEach(func() {
				header = &ModbusHeader{
					Slave:                    1,
					FunctionCode:             2,
					Address:                  50,
					NumberOfCoilsOrRegisters: 16,
				}
				input = []bool{
					// _ + 64 + 32 + _ + _ + _ + 2 + _ = 98
					false, true, true, false, false, false, true, false,
					// 128 + 64 + _ + _ + 8 + _ + 2 + 1 = 203
					true, true, false, false, true, false, true, true,
				}
			})
			It("equals the correct bytes", func() {
				Expect(result).To(Equal([]byte{
					1,  // slave
					0,  // msb function code
					2,  // lsb function code
					0,  // msb address
					50, // lsb address
					0,  // msb number of coils or registers
					16, // lsb number of coils or registers
					98,
					203,
				}))
			})
		})
		Context("with 9 coils", func() {
			BeforeEach(func() {
				header = &ModbusHeader{
					Slave:                    1,
					FunctionCode:             2,
					Address:                  50,
					NumberOfCoilsOrRegisters: 9,
				}
				input = []bool{
					// _ + 64 + 32 + _ + _ + _ + 2 + _ = 98
					false, true, true, false, false, false, true, false,
					// 128 + _ + _ + _ + _ + _ + _ + _ = 128
					true,
				}
			})
			It("equals the correct bytes", func() {
				Expect(result).To(Equal([]byte{
					1,  // slave
					0,  // msb function code
					2,  // lsb function code
					0,  // msb address
					50, // lsb address
					0,  // msb number of coils or registers
					9,  // lsb number of coils or registers
					98,
					128,
				}))
			})
		})
		Context("with 25 coils", func() {
			BeforeEach(func() {
				header = &ModbusHeader{
					Slave:                    1,
					FunctionCode:             2,
					Address:                  50,
					NumberOfCoilsOrRegisters: 25,
				}
				input = []bool{
					// _ + 64 + 32 + _ + _ + _ + 2 + _ = 98
					false, true, true, false, false, false, true, false,
					// 128 + 64 + _ + _ + 8 + _ + 2 + 1 = 203
					true, true, false, false, true, false, true, true,
					// _ + 64 + 32 + _ + _ + _ + 2 + _ = 98
					false, true, true, false, false, false, true, false,
					// 128 + _ + _ + _ + _ + _ + _ + _ = 128
					true,
				}
			})
			It("equals the correct bytes", func() {
				Expect(result).To(Equal([]byte{
					1,  // slave
					0,  // msb function code
					2,  // lsb function code
					0,  // msb address
					50, // lsb address
					0,  // msb number of coils or registers
					25, // lsb number of coils or registers
					98,
					203,
					98,
					128,
				}))
			})
		})
	})
})
