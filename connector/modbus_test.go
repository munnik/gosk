package connector_test

import (
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/connector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Helper functions", func() {
	Describe("BoolsToBytes", func() {
		var (
			rgc config.RegisterGroupConfig

			input  []bool
			result []byte
		)
		JustBeforeEach(func() {
			result = CoilsToBytes(rgc, input)
		})
		Context("with 8 coils", func() {
			BeforeEach(func() {
				rgc = config.RegisterGroupConfig{
					Slave:                  1,
					FunctionCode:           2,
					Address:                50,
					NumberOfCoilsRegisters: 8,
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
					0,
				}))
			})
		})
		Context("with 5 coils", func() {
			BeforeEach(func() {
				rgc = config.RegisterGroupConfig{
					Slave:                  1,
					FunctionCode:           2,
					Address:                50,
					NumberOfCoilsRegisters: 5,
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
					0,
				}))
			})
		})
		Context("with 16 coils", func() {
			BeforeEach(func() {
				rgc = config.RegisterGroupConfig{
					Slave:                  1,
					FunctionCode:           2,
					Address:                50,
					NumberOfCoilsRegisters: 16,
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
				rgc = config.RegisterGroupConfig{
					Slave:                  1,
					FunctionCode:           2,
					Address:                50,
					NumberOfCoilsRegisters: 9,
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
				rgc = config.RegisterGroupConfig{
					Slave:                  1,
					FunctionCode:           2,
					Address:                50,
					NumberOfCoilsRegisters: 25,
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
