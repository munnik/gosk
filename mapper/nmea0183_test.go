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
	m := "AIS: Antenna VSWR exceeds limit"
	f := false

	DescribeTable("Messages",
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
				m := message.NewRaw().WithConnector("testingConnector").WithType(config.NMEA0183Type).WithValue([]byte{})
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
				m := message.NewRaw().WithConnector("testingConnector").WithType(config.NMEA0183Type).WithValue([]byte("$AIALR,100615.00,002,V,V,AIS: Antenna VSWR exceeds limit*46"))
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.NMEA0183Type).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("notifications.ais").WithValue(message.Notification{State: &f, Message: &m}),
				),
			),
			false,
		),
		Entry("With an AIS message",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithConnector("testingConnector").WithType(config.NMEA0183Type).WithValue([]byte{0x21, 0x41, 0x49, 0x56, 0x44, 0x4d, 0x2c, 0x31, 0x2c, 0x31, 0x2c, 0x2c, 0x42, 0x2c, 0x33, 0x33, 0x63, 0x3a, 0x37, 0x32, 0x30, 0x30, 0x31, 0x47, 0x50, 0x45, 0x34, 0x53, 0x3c, 0x4d, 0x64, 0x45, 0x70, 0x34, 0x3b, 0x53, 0x4d, 0x3e, 0x30, 0x31, 0x34, 0x31, 0x2c, 0x30, 0x2a, 0x37, 0x36})
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			func() *message.Mapped {
				lat := 51.892
				lon := 4.60305
				mmsi := "246581000"
				m := message.NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:246581000").WithOrigin("testingContext").AddUpdate(
					message.NewUpdate().WithSource(
						*message.NewSource().WithLabel("testingConnector").WithType(config.NMEA0183Type).WithUuid(uuid.Nil),
					).WithTimestamp(
						now,
					).AddValue(
						message.NewValue().WithPath("").WithValue(message.VesselInfo{MMSI: &mmsi}),
					).AddValue(
						message.NewValue().WithPath("navigation.rateOfTurn").WithValue(0.0),
					).AddValue(
						message.NewValue().WithPath("navigation.courseOverGroundTrue").WithValue(1.8675022996339325),
					).AddValue(
						message.NewValue().WithPath("navigation.headingTrue").WithValue(1.9198621771937625),
					).AddValue(
						message.NewValue().WithPath("navigation.state").WithValue("motoring"),
					).AddValue(
						message.NewValue().WithPath("navigation.position").WithValue(message.Position{Latitude: &lat, Longitude: &lon}),
					).AddValue(
						message.NewValue().WithPath("navigation.speedOverGround").WithValue(4.475662799999999),
					),
				)
				return m
			}(),
			false,
		),
	)
})
