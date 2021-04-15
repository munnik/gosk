package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("VWR", func() {
	var (
		parsed VWR
	)
	Describe("Getting data from a $__VWR sentence", func() {
		BeforeEach(func() {
			parsed = VWR{
				Angle:                        NewFloat64(),
				WindSpeedInKnots:             NewFloat64(),
				WindSpeedInMetersPerSecond:   NewFloat64(),
				WindSpeedInKilometersPerHour: NewFloat64(),
			}
		})
		Context("When having a parsed sentence with missing data", func() {
			Specify("an error is returned", func() {
				_, err := parsed.GetRelativeWindDirection()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetWindSpeed()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
