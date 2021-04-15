package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

type VTG struct {
	goNMEA.BaseSentence
	TrueTrack        Float64
	MagneticTrack    Float64
	GroundSpeedKnots Float64
	GroundSpeedKPH   Float64
}

func init() {
	goNMEA.MustRegisterParser("VTG", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := VTG{
			BaseSentence: s,
		}
		if p.Fields[0] != "" {
			result.TrueTrack = NewFloat64(WithValue(p.Float64(0, "true track")))
		} else {
			result.TrueTrack = NewFloat64()
		}
		if p.Fields[2] != "" {
			result.MagneticTrack = NewFloat64(WithValue(p.Float64(2, "magnetic track")))
		} else {
			result.MagneticTrack = NewFloat64()
		}
		if p.Fields[4] != "" {
			result.GroundSpeedKnots = NewFloat64(WithValue(p.Float64(2, "magnetic track")))
		} else {
			result.GroundSpeedKnots = NewFloat64()
		}
		if p.Fields[6] != "" {
			result.GroundSpeedKPH = NewFloat64(WithValue(p.Float64(6, "ground speed (km/h)")))
		} else {
			result.GroundSpeedKPH = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetTrueCourseOverGround retrieves the true course over ground from the sentence
func (s VTG) GetTrueCourseOverGround() (float64, error) {
	if v, err := s.TrueTrack.GetValue(); err == nil {
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetMagneticCourseOverGround retrieves the magnetic course over ground from the sentence
func (s VTG) GetMagneticCourseOverGround() (float64, error) {
	if v, err := s.MagneticTrack.GetValue(); err == nil {
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetSpeedOverGround retrieves the speed over ground from the sentence
func (s VTG) GetSpeedOverGround() (float64, error) {
	if v, err := s.GroundSpeedKPH.GetValue(); err == nil {
		return (unit.Speed(v) * unit.KilometersPerHour).MetersPerSecond(), nil
	}
	if v, err := s.GroundSpeedKnots.GetValue(); err == nil {
		return (unit.Speed(v) * unit.Knot).MetersPerSecond(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}
