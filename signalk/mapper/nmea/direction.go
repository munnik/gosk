package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

// MagneticCourseOverGround retrieves the magnetic course over ground from the sentence
type MagneticCourseOverGround interface {
	GetmagneticCourseOverGround() (float64, uint32, error)
}

// MagneticHeading retrieves the magnetic heading from the sentence
type MagneticHeading interface {
	GetMagneticHeading() (float64, uint32, error)
}

// MagneticVariation retrieves the magnetic variation from the sentence
type MagneticVariation interface {
	GetMagneticVariation() (float64, uint32, error)
}

// TrueCourseOverGround retrieves the true course over ground from the sentence
type TrueCourseOverGround interface {
	GetTrueCourseOverGround() (float64, uint32, error)
}

// TrueHeading retrieves the true heading from the sentence
type TrueHeading interface {
	GetTrueHeading() (float64, uint32, error)
}

// GetMagneticHeading retrieves the magnetic heading from the sentence
func (s HDT) GetMagneticHeading() (float64, uint32, error) {
	if s.True {
		return 0, 0, fmt.Errorf("Heading is  true in sentence: %s", s)
	}
	return (unit.Angle(s.Heading) * unit.Degree).Radians(), 0, nil
}

// GetMagneticHeading retrieves the magnetic heading from the sentence
func (s VHW) GetMagneticHeading() (float64, uint32, error) {
	return (unit.Angle(s.MagneticHeading) * unit.Degree).Radians(), 0, nil
}

// GetMagneticVariation retrieves the magnetic variation from the sentence
func (s RMC) GetMagneticVariation() (float64, uint32, error) {
	if s.Validity != goNMEA.ValidRMC {
		return 0, 0, fmt.Errorf("The validity flag is set to %s in the sentence: %s", s.Validity, s)
	}
	return (unit.Angle(s.Variation) * unit.Degree).Radians(), 0, nil
}

// GetTrueCourseOverGround retrieves the true course over ground from the sentence
func (s RMC) GetTrueCourseOverGround() (float64, uint32, error) {
	if s.Validity != goNMEA.ValidRMC {
		return 0, 0, fmt.Errorf("The validity flag is set to %s in the sentence: %s", s.Validity, s)
	}
	return (unit.Angle(s.Course) * unit.Degree).Radians(), 0, nil
}

// GetTrueCourseOverGround retrieves the true course over ground from the sentence
func (s VTG) GetTrueCourseOverGround() (float64, uint32, error) {
	return (unit.Angle(s.TrueTrack) * unit.Degree).Radians(), 0, nil
}

// GetmagneticCourseOverGround retrieves the true course over ground from the sentence
func (s VTG) GetmagneticCourseOverGround() (float64, uint32, error) {
	return (unit.Angle(s.MagneticTrack) * unit.Degree).Radians(), 0, nil
}

// GetTrueHeading retrieves the true heading from the sentence
func (s HDT) GetTrueHeading() (float64, uint32, error) {
	if !s.True {
		return 0, 0, fmt.Errorf("Heading is not true in sentence: %s", s)
	}
	return (unit.Angle(s.Heading) * unit.Degree).Radians(), 0, nil
}

// GetTrueHeading retrieves the true heading from the sentence
func (s THS) GetTrueHeading() (float64, uint32, error) {
	if s.Status != goNMEA.AutonomousTHS {
		return 0, 0, fmt.Errorf("Heading status is not autonomous in sentence: %s", s)
	}
	return (unit.Angle(s.Heading) * unit.Degree).Radians(), 0, nil
}

// GetTrueHeading retrieves the true heading from the sentence
func (s VHW) GetTrueHeading() (float64, uint32, error) {
	return (unit.Angle(s.TrueHeading) * unit.Degree).Radians(), 0, nil
}
