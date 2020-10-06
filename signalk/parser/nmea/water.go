package nmea

import (
	"fmt"

	"github.com/martinlindhe/unit"
)

// DepthBelowSurface retrieves the depth below surface from the sentence
type DepthBelowSurface interface {
	GetDepthBelowSurface() (float64, error)
}

// DepthBelowTransducer retrieves the depth below the transducer from the sentence
type DepthBelowTransducer interface {
	GetDepthBelowTransducer() (float64, error)
}

// GetDepthBelowSurface retrieves the depth below surface from the sentence
func (s DBS) GetDepthBelowSurface() (float64, error) {
	if s.DepthMeters > 0 {
		return s.DepthMeters, nil
	}
	if s.DepthFeet > 0 {
		return (unit.Length(s.DepthFeet) * unit.Foot).Meters(), nil
	}
	if s.DepthFathoms > 0 {
		return (unit.Length(s.DepthFathoms) * unit.Fathom).Meters(), nil
	}
	return 0, fmt.Errorf("No depth found in sentence: %s", s)
}

// GetDepthBelowTransducer retrieves the depth below the transducer from the sentence
func (s DBT) GetDepthBelowTransducer() (float64, error) {
	if s.DepthMeters > 0 {
		return s.DepthMeters, nil
	}
	if s.DepthFeet > 0 {
		return (unit.Length(s.DepthFeet) * unit.Foot).Meters(), nil
	}
	if s.DepthFathoms > 0 {
		return (unit.Length(s.DepthFathoms) * unit.Fathom).Meters(), nil
	}
	return 0, fmt.Errorf("No depth found in sentence: %s", s)
}

// GetDepthBelowTransducer retrieves the depth below the transducer from the sentence
func (s DPT) GetDepthBelowTransducer() (float64, error) {
	return s.Depth, nil
}
