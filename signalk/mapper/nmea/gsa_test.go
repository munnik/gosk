package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	goNMEA "github.com/adrianmo/go-nmea"
	. "github.com/munnik/gosk/signalk/mapper/nmea"
)

var _ = Describe("GSA", func() {
	var (
		parsed GSA
	)
	Describe("Getting data from a $__GSA sentence", func() {
		BeforeEach(func() {
			parsed = GSA{
				Mode:    goNMEA.Auto,
				FixType: goNMEA.Fix3D,
				SV:      make([]string, Satellites),
				PDOP:    NewFloat64(),
				HDOP:    NewFloat64(),
				VDOP:    NewFloat64(),
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid number of satellites", func() {
				Expect(parsed.GetNumberOfSatellites()).To(Equal(Satellites))
			})
			It("should give a valid fix type", func() {
				Expect(parsed.GetFixType()).To(Equal(goNMEA.Fix3D))
			})
		})
	})
})
