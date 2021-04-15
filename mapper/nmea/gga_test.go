package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	goNMEA "github.com/adrianmo/go-nmea"
	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("GGA", func() {
	var (
		parsed GGA
	)
	Describe("Getting data from a $__GGA sentence", func() {
		BeforeEach(func() {
			parsed = GGA{
				Time:          goNMEA.Time{},
				Latitude:      NewFloat64(WithValue(Latitude)),
				Longitude:     NewFloat64(WithValue(Longitude)),
				FixQuality:    goNMEA.DGPS,
				NumSatellites: NewInt64(WithValue(Satellites)),
				HDOP:          NewFloat64(),
				Altitude:      NewFloat64(WithValue(Altitude)),
				Separation:    NewFloat64(),
				DGPSAge:       "",
				DGPSId:        "",
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid position", func() {
				lat, lon, alt, _ := parsed.GetPosition3D()
				Expect(lat).To(Equal(Latitude))
				Expect(lon).To(Equal(Longitude))
				Expect(alt).To(Equal(Altitude))
			})
			It("should give a valid number of satellites", func() {
				Expect(parsed.GetNumberOfSatellites()).To(Equal(Satellites))
			})
			It("should give a valid fix quality", func() {
				Expect(parsed.GetFixQuality()).To(Equal(goNMEA.DGPS))
			})
		})
		Context("When having a parsed sentence with a bad fix", func() {
			JustBeforeEach(func() {
				parsed.FixQuality = goNMEA.Invalid
			})
			Specify("an error is returned", func() {
				_, _, _, err := parsed.GetPosition3D()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid number of satellites", func() {
				Expect(parsed.GetNumberOfSatellites()).To(Equal(Satellites))
			})
			It("should give a valid fix quality", func() {
				Expect(parsed.GetFixQuality()).To(Equal(goNMEA.Invalid))
			})
		})
		Context("When having a parsed sentence with missing longitude", func() {
			JustBeforeEach(func() {
				parsed.Longitude = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, _, _, err := parsed.GetPosition3D()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid number of satellites", func() {
				Expect(parsed.GetNumberOfSatellites()).To(Equal(Satellites))
			})
			It("should give a valid fix quality", func() {
				Expect(parsed.GetFixQuality()).To(Equal(goNMEA.DGPS))
			})
		})
		Context("When having a parsed sentence with missing latitude", func() {
			JustBeforeEach(func() {
				parsed.Latitude = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, _, _, err := parsed.GetPosition3D()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid number of satellites", func() {
				Expect(parsed.GetNumberOfSatellites()).To(Equal(Satellites))
			})
			It("should give a valid fix quality", func() {
				Expect(parsed.GetFixQuality()).To(Equal(goNMEA.DGPS))
			})
		})
		Context("When having a parsed sentence with missing altitude", func() {
			JustBeforeEach(func() {
				parsed.Altitude = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, _, _, err := parsed.GetPosition3D()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid number of satellites", func() {
				Expect(parsed.GetNumberOfSatellites()).To(Equal(Satellites))
			})
			It("should give a valid fix quality", func() {
				Expect(parsed.GetFixQuality()).To(Equal(goNMEA.DGPS))
			})
		})
	})
})
