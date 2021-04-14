package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/signalk/mapper/nmea"
)

var _ = Describe("DBS", func() {
	var (
		parsed DBS
	)
	Describe("Getting data from a $__DBS sentence", func() {
		BeforeEach(func() {
			parsed = DBS{
				DepthFeet:    NewFloat64(WithValue(DepthBelowSurfaceFeet)),
				DepthMeters:  NewFloat64(WithValue(DepthBelowSurfaceMeters)),
				DepthFathoms: NewFloat64(WithValue(DepthBelowSurfaceFathoms)),
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid depth below surface", func() {
				Expect(parsed.GetDepthBelowSurface()).To(Float64Equal(DepthBelowSurfaceMeters, 0.00001))
			})
		})
		Context("When having a parsed sentence with only depth in feet set", func() {
			JustBeforeEach(func() {
				parsed.DepthMeters = NewFloat64()
				parsed.DepthFathoms = NewFloat64()
			})
			It("should give a valid depth below surface", func() {
				Expect(parsed.GetDepthBelowSurface()).To(Float64Equal(DepthBelowSurfaceMeters, 0.00001))
			})
		})
		Context("When having a parsed sentence with only depth in fathoms set", func() {
			JustBeforeEach(func() {
				parsed.DepthFeet = NewFloat64()
				parsed.DepthMeters = NewFloat64()
			})
			It("should give a valid depth below surface", func() {
				Expect(parsed.GetDepthBelowSurface()).To(Float64Equal(DepthBelowSurfaceMeters, 0.00001))
			})
		})
		Context("When having a parsed sentence with only depth in meters set", func() {
			JustBeforeEach(func() {
				parsed.DepthFeet = NewFloat64()
				parsed.DepthFathoms = NewFloat64()
			})
			It("should give a valid depth below surface", func() {
				Expect(parsed.GetDepthBelowSurface()).To(Float64Equal(DepthBelowSurfaceMeters, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing depth values", func() {
			JustBeforeEach(func() {
				parsed.DepthFeet = NewFloat64()
				parsed.DepthMeters = NewFloat64()
				parsed.DepthFathoms = NewFloat64()
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDepthBelowSurface()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
