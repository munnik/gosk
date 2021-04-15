package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("MDA", func() {
	var (
		parsed MDA
	)
	Describe("Getting data from a $__MDA sentence", func() {
		BeforeEach(func() {
			parsed = MDA{
				BarometricPressureInInchesOfMercury: NewFloat64(),
				BarometricPressureInBar:             NewFloat64(),
				AirTemperature:                      NewFloat64(),
				WaterTemperature:                    NewFloat64(),
				RelativeHumidity:                    NewFloat64(),
				AbsoluteHumidity:                    NewFloat64(),
				DewPoint:                            NewFloat64(),
				WindDirectionTrue:                   NewFloat64(),
				WindDirectionMagnetic:               NewFloat64(),
				WindSpeedInKnots:                    NewFloat64(),
				WindSpeedInMetersPerSecond:          NewFloat64(),
			}
		})
		Context("When having a parsed sentence with missing data", func() {
			Specify("an error is returned", func() {
				_, err := parsed.GetDewPointTemperature()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetHumidity()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetMagneticWindDirection()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetOutsidePressure()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetOutsideTemperature()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetTrueWindDirection()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetWindSpeed()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
