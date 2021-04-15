package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("MWV", func() {
	var (
		parsed MWV
	)
	Describe("Getting data from a $__MWV sentence", func() {
		BeforeEach(func() {
			parsed = MWV{
				Angle:         NewFloat64WithValue(RelativeDirectionDegrees),
				Reference:     ReferenceRelative,
				WindSpeed:     NewFloat64WithValue(SpeedOverGroundMPS),
				WindSpeedUnit: WindSpeedUnitMPS,
				Status:        ValidMWV,
			}
		})
		Context("When having a parsed sentence with reference set to relative", func() {
			It("should give a valid relative wind direction", func() {
				Expect(parsed.GetRelativeWindDirection()).To(Float64Equal(RelativeDirectionRadians, 0.00001))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetTrueWindDirection()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid wind speed", func() {
				Expect(parsed.GetWindSpeed()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with reference set to true", func() {
			JustBeforeEach(func() {
				parsed.Angle = NewFloat64WithValue(TrueDirectionDegrees)
				parsed.Reference = ReferenceTrue
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetRelativeWindDirection()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid true wind direction", func() {
				Expect(parsed.GetTrueWindDirection()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
		})
		Context("When having a parsed sentence with wind speed in kmh", func() {
			JustAfterEach(func() {
				parsed.WindSpeed = NewFloat64WithValue(SpeedOverGroundKPH)
				parsed.WindSpeedUnit = WindSpeedUnitKPH
			})
			It("should give a valid wind speed", func() {
				Expect(parsed.GetWindSpeed()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with wind speed in knots", func() {
			JustAfterEach(func() {
				parsed.WindSpeed = NewFloat64WithValue(SpeedOverGroundKnots)
				parsed.WindSpeedUnit = WindSpeedUnitKnots
			})
			It("should give a valid wind speed", func() {
				Expect(parsed.GetWindSpeed()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with status set to invalid", func() {
			JustBeforeEach(func() {
				parsed.Status = ""
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetRelativeWindDirection()
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
		Context("When having a parsed sentence with missing data", func() {
			JustBeforeEach(func() {
				parsed = MWV{}
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetRelativeWindDirection()
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
