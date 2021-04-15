package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

const (
	// TypeMWV for MWV messages
	TypeMWV = "MWV"

	ReferenceRelative = "R"
	ReferenceTrue     = "T"

	ValidMWV = "A"

	WindSpeedUnitKMH   = "K"
	WindSpeedUnitKnots = "N"
	WindSpeedUnitMPS   = "M"
)

// MWV Wind Speed and Angle
type MWV struct {
	goNMEA.BaseSentence
	Angle         Float64
	Reference     string
	WindSpeed     Float64
	WindSpeedUnit string
	Status        string
}

func init() {
	goNMEA.RegisterParser("MWV", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		p.AssertType(TypeMWV)
		result := MWV{
			BaseSentence:  s,
			Reference:     p.EnumString(1, "Reference", ReferenceRelative, ReferenceTrue),
			WindSpeedUnit: p.EnumString(3, "WindSpeedUnit"),
			Status:        p.EnumString(4, "Status", ValidMWV),
		}
		if p.Fields[0] != "" {
			result.Angle = NewFloat64(WithValue(p.Float64(0, "Angle")))
		} else {
			result.Angle = NewFloat64()
		}
		if p.Fields[2] != "" {
			result.WindSpeed = NewFloat64(WithValue(p.Float64(2, "WindSpeed")))
		} else {
			result.WindSpeed = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetTrueWindDirection retrieves the true wind direction from the sentence
func (s MWV) GetTrueWindDirection() (float64, error) {
	if s.Status == ValidMWV && s.Reference == ReferenceTrue {
		if v, err := s.Angle.GetValue(); err == nil {
			return (unit.Angle(v) * unit.Degree).Radians(), nil
		}
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetRelativeWindDirection retrieves the relative wind direction from the sentence
func (s MWV) GetRelativeWindDirection() (float64, error) {
	if s.Status == ValidMWV && s.Reference == ReferenceRelative {
		if v, err := s.Angle.GetValue(); err == nil {
			return (unit.Angle(v) * unit.Degree).Radians(), nil
		}
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetWindSpeed retrieves wind speed from the sentence
func (s MWV) GetWindSpeed() (float64, error) {
	if v, err := s.WindSpeed.GetValue(); err == nil && s.Status == ValidMWV {
		switch s.WindSpeedUnit {
		case WindSpeedUnitMPS:
			return v, nil
		case WindSpeedUnitKMH:
			return (unit.Speed(v) * unit.KilometersPerHour).MetersPerSecond(), nil
		case WindSpeedUnitKnots:
			return (unit.Speed(v) * unit.Knot).MetersPerSecond(), nil
		}
	}
	return 0, fmt.Errorf("value is unavailable")
}
