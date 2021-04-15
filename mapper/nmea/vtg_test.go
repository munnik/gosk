package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("VTG", func() {
	var (
		parsed VTG
	)
	Describe("Getting data from a $__VTG sentence", func() {
		BeforeEach(func() {
			parsed = VTG{
				TrueTrack:        NewFloat64WithValue(TrueDirectionDegrees),
				MagneticTrack:    NewFloat64WithValue(MagneticDirectionDegrees),
				GroundSpeedKPH:   NewFloat64WithValue(SpeedOverGroundKPH),
				GroundSpeedKnots: NewFloat64WithValue(SpeedOverGroundKnots),
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid true course over ground", func() {
				Expect(parsed.GetTrueCourseOverGround()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic course over ground", func() {
				Expect(parsed.GetMagneticCourseOverGround()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid speed over ground", func() {
				Expect(parsed.GetSpeedOverGround()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing true track", func() {
			JustBeforeEach(func() {
				parsed.TrueTrack = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetTrueCourseOverGround()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid magnetic course over ground", func() {
				Expect(parsed.GetMagneticCourseOverGround()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid speed over ground", func() {
				Expect(parsed.GetSpeedOverGround()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing magnetic track", func() {
			JustBeforeEach(func() {
				parsed.MagneticTrack = NewFloat64()
			})
			It("should give a valid true course over ground", func() {
				Expect(parsed.GetTrueCourseOverGround()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetMagneticCourseOverGround()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid speed over ground", func() {
				Expect(parsed.GetSpeedOverGround()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing speed over ground kph", func() {
			JustBeforeEach(func() {
				parsed.GroundSpeedKPH = NewFloat64()
			})
			It("should give a valid true course over ground", func() {
				Expect(parsed.GetTrueCourseOverGround()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic course over ground", func() {
				Expect(parsed.GetMagneticCourseOverGround()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid speed over ground", func() {
				Expect(parsed.GetSpeedOverGround()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing speed over ground knots", func() {
			JustBeforeEach(func() {
				parsed.GroundSpeedKnots = NewFloat64()
			})
			It("should give a valid true course over ground", func() {
				Expect(parsed.GetTrueCourseOverGround()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic course over ground", func() {
				Expect(parsed.GetMagneticCourseOverGround()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid speed over ground", func() {
				Expect(parsed.GetSpeedOverGround()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing speed over ground kph and knots", func() {
			JustBeforeEach(func() {
				parsed.GroundSpeedKPH = NewFloat64()
				parsed.GroundSpeedKnots = NewFloat64()
			})
			It("should give a valid true course over ground", func() {
				Expect(parsed.GetTrueCourseOverGround()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic course over ground", func() {
				Expect(parsed.GetMagneticCourseOverGround()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetSpeedOverGround()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
