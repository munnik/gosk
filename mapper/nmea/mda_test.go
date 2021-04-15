package nmea_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/munnik/gosk/mapper/nmea"
)

var _ = Describe("MDA", func() {
	var (
		parsed MDA
	)
	Describe("Getting data from a $__MDA sentence", func() {
		BeforeEach(func() {
			parsed = MDA{
				BarometricPressureInInchesOfMercury: NewFloat64WithValue(PressureInchesOfMercury),
				BarometricPressureInBar:             NewFloat64WithValue(PressureBar),
				AirTemperature:                      NewFloat64WithValue(AirTemperatureCelcius),
				WaterTemperature:                    NewFloat64WithValue(WaterTemperatureCelcius),
				RelativeHumidity:                    NewFloat64WithValue(RelativeHumidityPercentage),
				DewPoint:                            NewFloat64WithValue(DewPointCelcius),
				WindDirectionTrue:                   NewFloat64WithValue(TrueDirectionDegrees),
				WindDirectionMagnetic:               NewFloat64WithValue(MagneticDirectionDegrees),
				WindSpeedInKnots:                    NewFloat64WithValue(SpeedOverGroundKnots),
				WindSpeedInMetersPerSecond:          NewFloat64WithValue(SpeedOverGroundMPS),
			}
		})
		Context("When having a parsed sentence", func() {
			It("should give a valid outside pressure", func() {
				Expect(parsed.GetOutsidePressure()).To(Float64Equal(PressurePascal, 0.00001))
			})
			It("should give a valid outside temperature", func() {
				Expect(parsed.GetOutsideTemperature()).To(Float64Equal(AirTemperatureKelvin, 0.00001))
			})
			It("should give a valid water temperature", func() {
				Expect(parsed.GetWaterTemperature()).To(Float64Equal(WaterTemperatureKelvin, 0.00001))
			})
			It("should give a valid humidity", func() {
				Expect(parsed.GetHumidity()).To(Float64Equal(RelativeHumidityRatio, 0.00001))
			})
			It("should give a valid dew point temperature", func() {
				Expect(parsed.GetDewPointTemperature()).To(Float64Equal(DewPointKelvin, 0.00001))
			})
			It("should give a valid true wind direction", func() {
				Expect(parsed.GetTrueWindDirection()).To(Float64Equal(TrueDirectionRadians, 0.00001))
			})
			It("should give a valid magnetic wind direction", func() {
				Expect(parsed.GetMagneticWindDirection()).To(Float64Equal(MagneticDirectionRadians, 0.00001))
			})
			It("should give a valid wind speed", func() {
				Expect(parsed.GetWindSpeed()).To(Float64Equal(SpeedOverGroundMPS, 0.00001))
			})
		})
		Context("When having a parsed sentence with pressure inches of mercury missing", func() {
			JustAfterEach(func() {
				parsed.BarometricPressureInInchesOfMercury = NewFloat64()
			})
			It("should give a valid outside pressure", func() {
				Expect(parsed.GetOutsidePressure()).To(Float64Equal(PressurePascal, 0.00001))
			})
		})
		Context("When having a parsed sentence with pressure bar missing", func() {
			JustAfterEach(func() {
				parsed.BarometricPressureInBar = NewFloat64()
			})
			It("should give a valid outside pressure", func() {
				Expect(parsed.GetOutsidePressure()).To(Float64Equal(PressurePascal, 0.00001))
			})
		})
		Context("When having a parsed sentence with missing data", func() {
			JustBeforeEach(func() {
				parsed = MDA{}
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetDewPointTemperature()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetHumidity()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetMagneticWindDirection()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetOutsidePressure()
				Expect(err).To(HaveOccurred())
			})
			Specify("an error is returned", func() {
				_, err := parsed.GetOutsideTemperature()
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
