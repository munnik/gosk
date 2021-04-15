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
				Angle:                        NewFloat64WithValue(RelativeDirectionDegrees),
				WindSpeedInKnots:             NewFloat64WithValue(SpeedOverGroundKnots),
				WindSpeedInMetersPerSecond:   NewFloat64WithValue(SpeedOverGroundMPS),
				WindSpeedInKilometersPerHour: NewFloat64WithValue(SpeedOverGroundKPH),
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid relative wind direction", func() {
				Expect(parsed.GetRelativeWindDirection()).To(Float64Equal(RelativeDirectionRadians, 0.00001))
			})
			It("should give a valid wind speed", func() {
				Expect(parsed.GetWindSpeed()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing wind speed in meters per second", func() {
			JustBeforeEach(func() {
				parsed.WindSpeedInMetersPerSecond = NewFloat64()
			})
			It("should give a valid relative wind direction", func() {
				Expect(parsed.GetRelativeWindDirection()).To(Float64Equal(RelativeDirectionRadians, 0.00001))
			})
			It("should give a valid wind speed", func() {
				Expect(parsed.GetWindSpeed()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing wind speed in kilometer per hour", func() {
			JustBeforeEach(func() {
				parsed.WindSpeedInKilometersPerHour = NewFloat64()
			})
			It("should give a valid relative wind direction", func() {
				Expect(parsed.GetRelativeWindDirection()).To(Float64Equal(RelativeDirectionRadians, 0.00001))
			})
			It("should give a valid wind speed", func() {
				Expect(parsed.GetWindSpeed()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing wind speed in knots", func() {
			JustBeforeEach(func() {
				parsed.WindSpeedInKnots = NewFloat64()
			})
			It("should give a valid relative wind direction", func() {
				Expect(parsed.GetRelativeWindDirection()).To(Float64Equal(RelativeDirectionRadians, 0.00001))
			})
			It("should give a valid wind speed", func() {
				Expect(parsed.GetWindSpeed()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing data", func() {
			JustBeforeEach(func() {
				parsed = VWR{}
			})
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
