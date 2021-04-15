package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

type VHW struct {
	goNMEA.BaseSentence
	TrueHeading            Float64
	MagneticHeading        Float64
	SpeedThroughWaterKnots Float64
	SpeedThroughWaterKPH   Float64
}

func init() {
	goNMEA.MustRegisterParser("VHW", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := VHW{
			BaseSentence: s,
		}
		if p.Fields[0] != "" {
			result.TrueHeading = NewFloat64WithValue(p.Float64(0, "true heading"))
		} else {
			result.TrueHeading = NewFloat64()
		}
		if p.Fields[2] != "" {
			result.MagneticHeading = NewFloat64WithValue(p.Float64(2, "magnetic heading"))
		} else {
			result.MagneticHeading = NewFloat64()
		}
		if p.Fields[4] != "" {
			result.SpeedThroughWaterKnots = NewFloat64WithValue(p.Float64(4, "speed through water in knots"))
		} else {
			result.SpeedThroughWaterKnots = NewFloat64()
		}
		if p.Fields[6] != "" {
			result.SpeedThroughWaterKPH = NewFloat64WithValue(p.Float64(6, "speed through water in kilometers per hour"))
		} else {
			result.SpeedThroughWaterKPH = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetMagneticHeading retrieves the magnetic heading from the sentence
func (s VHW) GetMagneticHeading() (float64, error) {
	if v, err := s.MagneticHeading.GetValue(); err == nil {
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetTrueHeading retrieves the true heading from the sentence
func (s VHW) GetTrueHeading() (float64, error) {
	if v, err := s.TrueHeading.GetValue(); err == nil {
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetSpeedThroughWater retrieves the speed through water from the sentence
func (s VHW) GetSpeedThroughWater() (float64, error) {
	if v, err := s.SpeedThroughWaterKPH.GetValue(); err == nil {
		return (unit.Speed(v) * unit.KilometersPerHour).MetersPerSecond(), nil
	}
	if v, err := s.SpeedThroughWaterKnots.GetValue(); err == nil {
		return (unit.Speed(v) * unit.Knot).MetersPerSecond(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}
