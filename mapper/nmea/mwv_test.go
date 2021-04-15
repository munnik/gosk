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
				Angle:     NewFloat64(),
				WindSpeed: NewFloat64(),
			}
		})
		Context("When having a parsed sentence with missing data", func() {
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
