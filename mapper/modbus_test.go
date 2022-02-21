package mapper_test

import (
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Helper functions", func() {
	DescribeTable("RegistersToCoils",
		func(input []uint16, expected []bool) {
			result := RegistersToCoils(input)
			Expect(result).To(Equal(expected))
		},
		Entry("One register with value 0x0000", []uint16{0x0000}, []bool{
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
		Entry("One register with value 0x0001", []uint16{0x0001}, []bool{
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
			true,
		}),
		Entry("Three registers with value 0x45a3 0x7812 0x0001", []uint16{0x45a3, 0x7812, 0x0001}, []bool{
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
			true,
		}),
	)
})

var _ = Describe("DoMap Modbus", func() {
	mapper, _ := NewModbusMapper(
		config.MapperConfig{Context: "testingContext"},
		[]config.RegisterMappingConfig{
			{
				Slave:         1,
				FunctionCode:  config.DiscreteInputs,
				Address:       40,
				NumberOfCoils: 2,
				Expression:    "coils[0] && coils[1]",
				Path:          "testingPath",
			},
		},
	)
	now := time.Now()

	DescribeTable("Coils",
		func(m *ModbusMapper, input *message.Raw, expected *message.Mapped, expectError bool) {
			result, err := m.DoMap(input)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expected))
			}
		},
		Entry("With empty value",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{})
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			nil,
			true,
		),
		Entry("With invalid value",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{0, 5, 34, 4})
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			nil,
			true,
		),
		Entry("With value without registers",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{1, 0, 2, 0, 40, 0, 1})
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			nil,
			true,
		),
		Entry("With value all coils set to false",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{1, 0, 2, 0, 40, 0, 1, 0, 0})
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					message.NewSource().WithLabel("testingCollector").WithType(config.ModbusType),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("testingPath").WithUuid(uuid.Nil).WithValue(false),
				),
			),
			false,
		),
		Entry("With value all coils set to true",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{1, 0, 2, 0, 40, 0, 1, 255, 255})
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					message.NewSource().WithLabel("testingCollector").WithType(config.ModbusType),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("testingPath").WithUuid(uuid.Nil).WithValue(true),
				),
			),
			false,
		),
	)
})
