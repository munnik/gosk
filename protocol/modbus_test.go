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
		func(input []uint16, expected []bool) {
			result := protocol.RegistersToCoils(input)
			Expect(result).To(Equal(expected))
		},
		Entry("One register", []uint16{0xff00}, []bool{
			true,
		}),
		Entry("Five register", []uint16{0xff00, 0x0000, 0x0000, 0xff00, 0x0000}, []bool{
			true,
			false,
			false,
			true,
			false,
		}),
	)
	DescribeTable(
		"CoilsToRegisters",
		func(expected []uint16, input []bool) {
			result := protocol.CoilsToRegisters(input)
			Expect(result).To(Equal(expected))
		},
		Entry("2 coils, both false", []uint16{0x0000, 0x0000}, []bool{
			false,
			false,
		}),
		Entry("3 coils, first true rest false", []uint16{0xff00, 0x0000, 0x0000}, []bool{
			true,
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
					Slave:                    0x01,
					FunctionCode:             0x02,
					Address:                  0x50,
					NumberOfCoilsOrRegisters: 0x08,
				}
				input = []bool{true, true, false, false, true, false, true, true}
			})
			It("equals the correct bytes", func() {
				Expect(result).To(Equal([]byte{
					0x01, // slave
					0x00, // msb function code
					0x02, // lsb function code
					0x00, // msb address
					0x50, // lsb address
					0x00, // msb number of coils or registers
					0x08, // lsb number of coils or registers
					0xff, // true
					0x00,
					0xff, // true
					0x00,
					0x00, // false
					0x00,
					0x00, // false
					0x00,
					0xff, // true
					0x00,
					0x00, // false
					0x00,
					0xff, // true
					0x00,
					0xff, // true
					0x00,
				}))
			})
		})
		Context("with 5 coils", func() {
			BeforeEach(func() {
				header = &ModbusHeader{
					Slave:                    0x01,
					FunctionCode:             0x02,
					Address:                  0x50,
					NumberOfCoilsOrRegisters: 0x05,
				}
				input = []bool{true, true, false, false, true}
			})
			It("equals the correct bytes", func() {
				Expect(result).To(Equal([]byte{
					0x01, // slave
					0x00, // msb function code
					0x02, // lsb function code
					0x00, // msb address
					0x50, // lsb address
					0x00, // msb number of coils or registers
					0x05, // lsb number of coils or registers
					0xff, // true
					0x00,
					0xff, // true
					0x00,
					0x00, // false
					0x00,
					0x00, // false
					0x00,
					0xff, // true
					0x00,
				}))
			})
		})
	})
})
