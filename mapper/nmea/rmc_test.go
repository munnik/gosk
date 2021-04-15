package nmea_test

import (
	goNMEA "github.com/adrianmo/go-nmea"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("RMC", func() {
	var (
		parsed RMC
	)
	Describe("Getting directions from a $__RMC sentence", func() {
		BeforeEach(func() {
			parsed = RMC{
				Time:      goNMEA.Time{},
				Validity:  goNMEA.ValidRMC,
				Latitude:  NewFloat64WithValue(Latitude),
				Longitude: NewFloat64WithValue(Longitude),
				Speed:     NewFloat64WithValue(SpeedOverGroundKnots),
				Course:    NewFloat64WithValue(TrueDirectionDegrees),
				Variation: NewFloat64WithValue(MagneticVariationDegrees),
				Date:      goNMEA.Date{},
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid position", func() {
				lat, lon, _ := parsed.GetPosition2D()
				Expect(lat).To(Equal(Latitude))
				Expect(lon).To(Equal(Longitude))
			})
			It("should give a valid true course over ground", func() {
				Expect(parsed.GetTrueCourseOverGround()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic variation", func() {
				Expect(parsed.GetMagneticVariation()).To(Float64Equal(MagneticVariationRadians, 0.00001))
			})
		})
		Context("When having a parsed sentence with the validity flag set to invalid", func() {
			JustBeforeEach(func() {
				parsed.Validity = goNMEA.InvalidRMC
			})
			Specify("an error is returned when trying to retrieve the true course over ground", func() {
				value, err := parsed.GetTrueCourseOverGround()
				Expect(value).To(BeZero())
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned when trying to retrieve the magnetic variation", func() {
				value, err := parsed.GetMagneticVariation()
				Expect(value).To(BeZero())
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
