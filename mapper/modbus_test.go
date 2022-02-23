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
		config.NewModbusMappingsConfig("modbus_test.yaml"),
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
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{1, 0, 2, 0, 40, 0, 2, 0, 0})
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
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{1, 0, 2, 0, 40, 0, 2, 192, 0})
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
		Entry("With real data",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{2, 0, 2, 3, 32, 0, 8, 255, 0})
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
					message.NewValue().WithPath("notifications.tanks.fuel.portAft").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "The fuel level is too high"}),
				).AddValue(
					message.NewValue().WithPath("notifications.tanks.fuel.portAft").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "The fuel level is too low"}),
				).AddValue(
					message.NewValue().WithPath("notifications.tanks.fuel.starboardAft").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "The fuel level is too high"}),
				).AddValue(
					message.NewValue().WithPath("notifications.tanks.fuel.starboardAft").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "The fuel level is too low"}),
				).AddValue(
					message.NewValue().WithPath("notifications.bilge.engineRoomForward").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "The bilge level is too high"}),
				).AddValue(
					message.NewValue().WithPath("notifications.bilge.hold1").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "The bilge level is too high"}),
				).AddValue(
					message.NewValue().WithPath("notifications.bilge.hold2").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "The bilge level is too high"}),
				).AddValue(
					message.NewValue().WithPath("notifications.bilge.engineRoomAft").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "The bilge level is too high"}),
				),
			),
			false,
		),
	)
	DescribeTable("Holding registers",
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
		Entry("With value and actual registers",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{1, 0, 3, 0, 52, 0, 1, 15, 146})
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
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate").WithUuid(uuid.Nil).WithValue(-1.9444444444444323e-08),
				),
			),
			false,
		),
		Entry("With two registers",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.ModbusType).WithValue([]byte{1, 0, 4, 0, 22, 0, 2, 15, 146, 67, 234})
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
					message.NewValue().WithPath("testingPath").WithUuid(uuid.Nil).WithValue(261243882),
				),
			),
			false,
		),
	)
})
