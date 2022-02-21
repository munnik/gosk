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

var _ = Describe("DoMap nmea0183", func() {
	mapper, _ := NewNmea0183Mapper(
		config.MapperConfig{Context: "testingContext"},
	)
	now := time.Now()

	DescribeTable("Coils",
		func(m *Nmea0183Mapper, input *message.Raw, expected *message.Mapped, expectError bool) {
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
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.NMEA0183Type).WithValue([]byte{})
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			nil,
			true,
		),
		Entry("With an AIS alarm message",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.NMEA0183Type).WithValue([]byte("$AIALR,100615.00,002,V,V,AIS: Antenna VSWR exceeds limit*46"))
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					message.NewSource().WithLabel("testingCollector").WithType(config.NMEA0183Type),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("notifications.ais").WithUuid(uuid.Nil).WithValue(message.Alarm{State: false, Message: "AIS: Antenna VSWR exceeds limit"}),
				),
			),
			false,
		),
	)
})
