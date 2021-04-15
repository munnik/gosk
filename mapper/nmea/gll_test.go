package nmea_test

import (
	goNMEA "github.com/adrianmo/go-nmea"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("GLL", func() {
	var (
		parsed GLL
	)
	Describe("Getting data from a $__GLL sentence", func() {
		BeforeEach(func() {
			parsed = GLL{
				Time:      goNMEA.Time{},
				Latitude:  NewFloat64WithValue(Latitude),
				Longitude: NewFloat64WithValue(Longitude),
				Validity:  goNMEA.ValidGLL,
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid position", func() {
				lat, lon, _ := parsed.GetPosition2D()
				Expect(lat).To(Equal(Latitude))
				Expect(lon).To(Equal(Longitude))
			})
		})
		Context("When having a parsed sentence with validity set to invalid", func() {
			JustBeforeEach(func() {
				parsed.Validity = goNMEA.InvalidGLL
			})
			Specify("an error is returned", func() {
				_, _, err := parsed.GetPosition2D()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("When having a parsed sentence with missing longitude", func() {
			JustBeforeEach(func() {
				parsed.Longitude = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, _, err := parsed.GetPosition2D()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("When having a parsed sentence with missing latitude", func() {
			JustBeforeEach(func() {
				parsed.Latitude = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, _, err := parsed.GetPosition2D()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
