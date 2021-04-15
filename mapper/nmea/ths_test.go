package nmea_test

import (
	goNMEA "github.com/adrianmo/go-nmea"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("THS", func() {
	var (
		parsed THS
	)
	Describe("Getting data from a $__THS sentence", func() {
		BeforeEach(func() {
			parsed = THS{
				Heading: NewFloat64WithValue(TrueDirectionDegrees),
				Status:  goNMEA.SimulatorTHS,
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
		Context("When having a parsed sentence with status flag set to invalid", func() {
			JustBeforeEach(func() {
				parsed.Status = goNMEA.InvalidTHS
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetTrueHeading()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
