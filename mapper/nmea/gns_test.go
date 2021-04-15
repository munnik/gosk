package nmea_test

import (
	goNMEA "github.com/adrianmo/go-nmea"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("GNS", func() {
	var (
		parsed GNS
	)
	Describe("Getting data from a $__GNS sentence", func() {
		BeforeEach(func() {
			parsed = GNS{
				Time:       goNMEA.Time{},
				Latitude:   NewFloat64(WithValue(Latitude)),
				Longitude:  NewFloat64(WithValue(Longitude)),
				Mode:       []string{goNMEA.SimulatorGNS},
				SVs:        NewInt64(),
				HDOP:       NewFloat64(),
				Altitude:   NewFloat64(WithValue(Altitude)),
				Separation: NewFloat64(),
				Age:        NewFloat64(),
				Station:    NewInt64(),
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid position", func() {
				lat, lon, alt, _ := parsed.GetPosition3D()
				Expect(lat).To(Equal(Latitude))
				Expect(lon).To(Equal(Longitude))
				Expect(alt).To(Equal(Altitude))
			})
		})
	})
})
