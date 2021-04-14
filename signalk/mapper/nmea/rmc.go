package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

type RMC struct {
	goNMEA.BaseSentence
	Time      goNMEA.Time // Time Stamp
	Validity  string      // validity - A-ok, V-invalid
	Latitude  Float64     // Latitude
	Longitude Float64     // Longitude
	Speed     Float64     // Speed in knots
	Course    Float64     // True course
	Date      goNMEA.Date // Date
	Variation Float64     // Magnetic variation
}

func init() {
	goNMEA.MustRegisterParser("RMC", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := RMC{
			BaseSentence: s,
			Time:         p.Time(0, "time"),
			Validity:     p.EnumString(1, "validity", goNMEA.ValidRMC, goNMEA.InvalidRMC),
			Date:         p.Date(8, "date"),
		}
		if p.Fields[2] != "" && p.Fields[3] != "" {
			result.Latitude = NewFloat64(WithValue(p.LatLong(2, 3, "latitude")))
		} else {
			result.Latitude = NewFloat64()
		}
		if p.Fields[4] != "" && p.Fields[5] != "" {
			result.Longitude = NewFloat64(WithValue(p.LatLong(4, 5, "longitude")))
		} else {
			result.Longitude = NewFloat64()
		}
		if p.Fields[6] != "" {
			result.Speed = NewFloat64(WithValue(p.Float64(6, "speed")))
		} else {
			result.Speed = NewFloat64()
		}
		if p.Fields[7] != "" {
			result.Course = NewFloat64(WithValue(p.Float64(7, "course")))
		} else {
			result.Course = NewFloat64()
		}
		if p.Fields[9] != "" {
			result.Variation = NewFloat64(WithValue(p.Float64(9, "variation")))
		} else {
			result.Variation = NewFloat64()
		}
		if !result.Variation.isNil && p.EnumString(10, "direction", goNMEA.West, goNMEA.East) == goNMEA.West {
			result.Variation.value = 0 - result.Variation.value
		}
		return result, p.Err()
	})
}

// GetMagneticVariation retrieves the magnetic variation from the sentence
func (s RMC) GetMagneticVariation() (float64, error) {
	if !s.Variation.isNil && s.Validity == goNMEA.ValidRMC {
		return (unit.Angle(s.Variation.value) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetTrueCourseOverGround retrieves the true course over ground from the sentence
func (s RMC) GetTrueCourseOverGround() (float64, error) {
	if !s.Course.isNil && s.Validity == goNMEA.ValidRMC {
		return (unit.Angle(s.Course.value) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetPosition2D retrieves the latitude and longitude from the sentence
func (s RMC) GetPosition2D() (float64, float64, error) {
	if !s.Latitude.isNil && !s.Longitude.isNil && s.Validity == goNMEA.ValidRMC {
		return s.Latitude.value, s.Longitude.value, nil
	}
	return 0, 0, fmt.Errorf("value is unavailable")
}

// GetSpeedOverGround retrieves the speed over ground from the sentence
func (s RMC) GetSpeedOverGround() (float64, error) {
	if !s.Longitude.isNil && s.Validity == goNMEA.ValidRMC {
		return (unit.Speed(s.Speed.value) * unit.Knot).MetersPerSecond(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}
