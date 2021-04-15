package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

const (
	// TypeMDA for MDA messages
	TypeMDA = "MDA"
)

// MDA Meteorological Composite
type MDA struct {
	goNMEA.BaseSentence
	BarometricPressureInInchesOfMercury Float64
	BarometricPressureInBar             Float64
	AirTemperature                      Float64
	WaterTemperature                    Float64
	RelativeHumidity                    Float64
	DewPoint                            Float64
	WindDirectionTrue                   Float64
	WindDirectionMagnetic               Float64
	WindSpeedInKnots                    Float64
	WindSpeedInMetersPerSecond          Float64
}

func init() {
	goNMEA.RegisterParser("MDA", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		p.AssertType(TypeMDA)
		result := MDA{
			BaseSentence: s,
		}
		if p.Fields[0] != "" {
			result.BarometricPressureInInchesOfMercury = NewFloat64WithValue(p.Float64(0, "BarometricPressureInInchesOfMercury"))
		} else {
			result.BarometricPressureInInchesOfMercury = NewFloat64()
		}
		if p.Fields[2] != "" {
			result.BarometricPressureInBar = NewFloat64WithValue(p.Float64(2, "BarometricPressureInBar"))
		} else {
			result.BarometricPressureInBar = NewFloat64()
		}
		if p.Fields[4] != "" {
			result.AirTemperature = NewFloat64WithValue(p.Float64(4, "AirTemperature"))
		} else {
			result.AirTemperature = NewFloat64()
		}
		if p.Fields[6] != "" {
			result.WaterTemperature = NewFloat64WithValue(p.Float64(6, "WaterTemperature"))
		} else {
			result.WaterTemperature = NewFloat64()
		}
		if p.Fields[8] != "" {
			result.RelativeHumidity = NewFloat64WithValue(p.Float64(8, "RelativeHumidity"))
		} else {
			result.RelativeHumidity = NewFloat64()
		}
		if p.Fields[10] != "" {
			result.DewPoint = NewFloat64WithValue(p.Float64(10, "DewPoint"))
		} else {
			result.DewPoint = NewFloat64()
		}
		if p.Fields[12] != "" {
			result.WindDirectionTrue = NewFloat64WithValue(p.Float64(12, "WindDirectionTrue"))
		} else {
			result.WindDirectionTrue = NewFloat64()
		}
		if p.Fields[14] != "" {
			result.WindDirectionMagnetic = NewFloat64WithValue(p.Float64(14, "WindDirectionMagnetic"))
		} else {
			result.WindDirectionMagnetic = NewFloat64()
		}
		if p.Fields[16] != "" {
			result.WindSpeedInKnots = NewFloat64WithValue(p.Float64(16, "WindSpeedInKnots"))
		} else {
			result.WindSpeedInKnots = NewFloat64()
		}
		if p.Fields[18] != "" {
			result.WindSpeedInMetersPerSecond = NewFloat64WithValue(p.Float64(16, "WindSpeedInMetersPerSecond"))
		} else {
			result.WindSpeedInMetersPerSecond = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetTrueWindDirection retrieves the true wind direction from the sentence
func (s MDA) GetTrueWindDirection() (float64, error) {
	if v, err := s.WindDirectionTrue.GetValue(); err == nil {
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetMagneticWindDirection retrieves the true wind direction from the sentence
func (s MDA) GetMagneticWindDirection() (float64, error) {
	if v, err := s.WindDirectionMagnetic.GetValue(); err == nil {
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetWindSpeed retrieves wind speed from the sentence
func (s MDA) GetWindSpeed() (float64, error) {
	if v, err := s.WindSpeedInMetersPerSecond.GetValue(); err == nil {
		return v, nil
	}
	if v, err := s.WindSpeedInKnots.GetValue(); err == nil {
		return (unit.Speed(v) * unit.Knot).MetersPerSecond(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetOutsideTemperature retrieves the outside air temperature from the sentence
func (s MDA) GetOutsideTemperature() (float64, error) {
	if v, err := s.AirTemperature.GetValue(); err == nil {
		return unit.FromCelsius(v).Kelvin(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetOutsideTemperature retrieves the outside air temperature from the sentence
func (s MDA) GetWaterTemperature() (float64, error) {
	if v, err := s.WaterTemperature.GetValue(); err == nil {
		return unit.FromCelsius(v).Kelvin(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetDewPointTemperature retrieves the dew point temperature from the sentence
func (s MDA) GetDewPointTemperature() (float64, error) {
	if v, err := s.DewPoint.GetValue(); err == nil {
		return unit.FromCelsius(v).Kelvin(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetOutsidePressure retrieves the outside pressure from the sentence
func (s MDA) GetOutsidePressure() (float64, error) {
	if v, err := s.BarometricPressureInBar.GetValue(); err == nil {
		return (unit.Pressure(v) * unit.Bar).Pascals(), nil
	}
	if v, err := s.BarometricPressureInInchesOfMercury.GetValue(); err == nil {
		return (unit.Pressure(v) * unit.InchOfMercury).Pascals(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetHumidity retrieves the relative humidity from the sentence
func (s MDA) GetHumidity() (float64, error) {
	if v, err := s.RelativeHumidity.GetValue(); err == nil {
		return v / 100.0, nil
	}
	return 0, fmt.Errorf("value is unavailable")
}
