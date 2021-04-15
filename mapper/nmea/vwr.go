package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

const (
	// TypeVWR for VWR messages
	TypeVWR = "VWR"

	LeftOfBow  = "L"
	RightOfBow = "R"
)

// VWR Relative Wind Speed and Angle
type VWR struct {
	goNMEA.BaseSentence
	Angle                        Float64
	LeftRightOfBow               string
	WindSpeedInKnots             Float64
	WindSpeedInMetersPerSecond   Float64
	WindSpeedInKilometersPerHour Float64
}

func init() {
	goNMEA.RegisterParser("VWR", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		p.AssertType(TypeVWR)
		result := VWR{
			BaseSentence:   s,
			LeftRightOfBow: p.EnumString(1, "LeftRightOfBow", LeftOfBow, RightOfBow),
		}
		if p.Fields[0] != "" {
			result.Angle = NewFloat64WithValue(p.Float64(0, "Angle"))
		} else {
			result.Angle = NewFloat64()
		}
		if p.Fields[2] != "" {
			result.WindSpeedInKnots = NewFloat64WithValue(p.Float64(2, "WindSpeedInKnots"))
		} else {
			result.WindSpeedInKnots = NewFloat64()
		}
		if p.Fields[4] != "" {
			result.WindSpeedInMetersPerSecond = NewFloat64WithValue(p.Float64(4, "WindSpeedInMetersPerSecond"))
		} else {
			result.WindSpeedInMetersPerSecond = NewFloat64()
		}
		if p.Fields[6] != "" {
			result.WindSpeedInKilometersPerHour = NewFloat64WithValue(p.Float64(6, "WindSpeedInKilometersPerHour"))
		} else {
			result.WindSpeedInKilometersPerHour = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetRelativeWindDirection retrieves the true wind direction from the sentence
func (s VWR) GetRelativeWindDirection() (float64, error) {
	if v, err := s.Angle.GetValue(); err == nil {
		if s.LeftRightOfBow == LeftOfBow {
			return -(unit.Angle(v) * unit.Degree).Radians(), nil
		}
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetWindSpeed retrieves wind speed from the sentence
func (s VWR) GetWindSpeed() (float64, error) {
	if v, err := s.WindSpeedInMetersPerSecond.GetValue(); err == nil {
		return v, nil
	}
	if v, err := s.WindSpeedInKilometersPerHour.GetValue(); err == nil {
		return (unit.Speed(v) * unit.KilometersPerHour).MetersPerSecond(), nil
	}
	if v, err := s.WindSpeedInKnots.GetValue(); err == nil {
		return (unit.Speed(v) * unit.Knot).MetersPerSecond(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}
