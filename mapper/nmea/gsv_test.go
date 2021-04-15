package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("GSV", func() {
	var (
		parsed GSV
	)
	Describe("Getting data from a $__GSV sentence", func() {
		BeforeEach(func() {
			parsed = GSV{
				NumberSVsInView: NewInt64WithValue(Satellites),
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid number of satellites", func() {
				Expect(parsed.GetNumberOfSatellites()).To(Equal(Satellites))
			})
		})
		Context("When having a parsed sentence without a number of satellites", func() {
			JustBeforeEach(func() {
				parsed.NumberSVsInView = NewInt64()
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetNumberOfSatellites()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
