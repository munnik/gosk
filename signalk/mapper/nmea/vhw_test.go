package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/signalk/mapper/nmea"
)

var _ = Describe("VHW", func() {
	var (
		parsed VHW
	)
	Describe("Getting data from a $__VHW sentence", func() {
		BeforeEach(func() {
			parsed = VHW{
				TrueHeading:            NewFloat64(WithValue(TrueDirectionDegrees)),
				MagneticHeading:        NewFloat64(WithValue(MagneticDirectionDegrees)),
				SpeedThroughWaterKPH:   NewFloat64(WithValue(SpeedThroughWaterKPH)),
				SpeedThroughWaterKnots: NewFloat64(WithValue(SpeedThroughWaterKnots)),
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid true heading", func() {
				Expect(parsed.GetTrueHeading()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic heading", func() {
				Expect(parsed.GetMagneticHeading()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid speed through water", func() {
				Expect(parsed.GetSpeedThroughWater()).To(Float64Equal(SpeedThroughWaterMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing true heading", func() {
			JustBeforeEach(func() {
				parsed.TrueHeading = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetTrueHeading()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid magnetic heading", func() {
				Expect(parsed.GetMagneticHeading()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid speed through water", func() {
				Expect(parsed.GetSpeedThroughWater()).To(Float64Equal(SpeedThroughWaterMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing magnetic track", func() {
			JustBeforeEach(func() {
				parsed.MagneticHeading = NewFloat64()
			})
			It("should give a valid true heading", func() {
				Expect(parsed.GetTrueHeading()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetMagneticHeading()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid speed through water", func() {
				Expect(parsed.GetSpeedThroughWater()).To(Float64Equal(SpeedThroughWaterMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing speed over ground kph", func() {
			JustBeforeEach(func() {
				parsed.SpeedThroughWaterKPH = NewFloat64()
			})
			It("should give a valid true heading", func() {
				Expect(parsed.GetTrueHeading()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic heading", func() {
				Expect(parsed.GetMagneticHeading()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid speed through water", func() {
				Expect(parsed.GetSpeedThroughWater()).To(Float64Equal(SpeedThroughWaterMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing speed over ground knots", func() {
			JustBeforeEach(func() {
				parsed.SpeedThroughWaterKnots = NewFloat64()
			})
			It("should give a valid true heading", func() {
				Expect(parsed.GetTrueHeading()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic heading", func() {
				Expect(parsed.GetMagneticHeading()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid speed through water", func() {
				Expect(parsed.GetSpeedThroughWater()).To(Float64Equal(SpeedThroughWaterMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing speed over ground kph and knots", func() {
			JustBeforeEach(func() {
				parsed.SpeedThroughWaterKPH = NewFloat64()
				parsed.SpeedThroughWaterKnots = NewFloat64()
			})
			It("should give a valid true heading", func() {
				Expect(parsed.GetTrueHeading()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic heading", func() {
				Expect(parsed.GetMagneticHeading()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetSpeedThroughWater()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
