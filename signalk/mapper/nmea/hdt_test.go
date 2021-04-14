package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/signalk/mapper/nmea"
)

var _ = Describe("HDT", func() {
	var (
		parsed HDT
	)
	Describe("Getting data from a $__HDT sentence", func() {
		BeforeEach(func() {
			parsed = HDT{
				Heading: NewFloat64(WithValue(TrueDirectionDegrees)),
				True:    true,
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid true heading", func() {
				Expect(parsed.GetTrueHeading()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing heading", func() {
			JustBeforeEach(func() {
				parsed.Heading = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetTrueHeading()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("When having a parsed sentence with true flag not set", func() {
			JustBeforeEach(func() {
				parsed.True = false
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetTrueHeading()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
