package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("DPT", func() {
	var (
		parsed DPT
	)
	Describe("Getting data from a $__DPT sentence", func() {
		BeforeEach(func() {
			parsed = DPT{
				Depth: NewFloat64(WithValue(DepthBelowSurfaceMeters - DepthTransducerMeters)),
			}
		})
		Context("When having a parsed sentence and a positive offset", func() {
			JustBeforeEach(func() {
				parsed.Offset = NewFloat64(WithValue(DepthTransducerMeters))
			})
			It("should give a valid depth below transducer", func() {
				Expect(parsed.GetDepthBelowTransducer()).To(Float64Equal(DepthBelowSurfaceMeters-DepthTransducerMeters, 0.00001))
			})
			It("should give a valid depth below surface", func() {
				Expect(parsed.GetDepthBelowSurface()).To(Float64Equal(DepthBelowSurfaceMeters, 0.00001))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDepthBelowKeel()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("When having a parsed sentence and a negative offset", func() {
			JustBeforeEach(func() {
				parsed.Offset = NewFloat64(WithValue(DepthTransducerMeters - DepthKeelMeters))
			})
			It("should give a valid depth below transducer", func() {
				Expect(parsed.GetDepthBelowTransducer()).To(Float64Equal(DepthBelowSurfaceMeters-DepthTransducerMeters, 0.00001))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDepthBelowSurface()
				Expect(err).To(HaveOccurred())
			})
			It("should give a valid depth below keel", func() {
				Expect(parsed.GetDepthBelowKeel()).To(Float64Equal(DepthBelowSurfaceMeters-DepthKeelMeters, 0.00001))
			})
		})
		Context("When having a parsed sentence and no offset", func() {
			JustBeforeEach(func() {
				parsed.Offset = NewFloat64()
			})
			It("should give a valid depth below transducer", func() {
				Expect(parsed.GetDepthBelowTransducer()).To(Float64Equal(DepthBelowSurfaceMeters-DepthTransducerMeters, 0.00001))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDepthBelowSurface()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDepthBelowKeel()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("When having a parsed sentence and no depth", func() {
			JustBeforeEach(func() {
				parsed.Depth = NewFloat64()
				parsed.Offset = NewFloat64(WithValue(DepthTransducerMeters))
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDepthBelowTransducer()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDepthBelowSurface()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDepthBelowKeel()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
