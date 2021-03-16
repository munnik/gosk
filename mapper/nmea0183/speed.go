package nmea0183

import (
	"errors"
	"fmt"

	goAIS "github.com/BertoldVdb/go-ais"
	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

// SpeedOverGround retrieves the speed over ground from the sentence
type SpeedOverGround interface {
	GetSpeedOverGround() (float64, error)
}

// SpeedThroughWater retrieves the speed through water from the sentence
type SpeedThroughWater interface {
	GetSpeedThroughWater() (float64, error)
}

// GetSpeedOverGround retrieves the speed over ground from the sentence
func (s RMC) GetSpeedOverGround() (float64, error) {
	if s.Validity != goNMEA.ValidRMC {
		return 0.0, fmt.Errorf("The invalid flag is set to %s in the sentence: %s", s.Validity, s)
	}
	return (unit.Speed(s.Speed) * unit.Knot).MetersPerSecond(), nil
}

// GetSpeedOverGround retrieves the speed over ground from the sentence
func (s VTG) GetSpeedOverGround() (float64, error) {
	if s.GroundSpeedKPH > 0 {
		return (unit.Speed(s.GroundSpeedKPH) * unit.KilometersPerHour).MetersPerSecond(), nil
	}
	if s.GroundSpeedKnots > 0 {
		return (unit.Speed(s.GroundSpeedKnots) * unit.Knot).MetersPerSecond(), nil
	}
	return 0.0, nil
}

// GetSpeedOverGround retrieves the speed over ground from the sentence
func (s VDMVDO) GetSpeedOverGround() (float64, error) {
	if positionReport, ok := s.Packet.(goAIS.PositionReport); ok && positionReport.Valid {
		return (unit.Speed(positionReport.Sog) * unit.Knot).MetersPerSecond(), nil
	}
	return 0.0, errors.New("Not a position report or invalid position report")
}

// GetSpeedThroughWater retrieves the speed through water from the sentence
func (s VHW) GetSpeedThroughWater() (float64, error) {
	if s.SpeedThroughWaterKPH > 0 {
		return (unit.Speed(s.SpeedThroughWaterKPH) * unit.KilometersPerHour).MetersPerSecond(), nil
	}
	if s.SpeedThroughWaterKnots > 0 {
		return (unit.Speed(s.SpeedThroughWaterKnots) * unit.Knot).MetersPerSecond(), nil
	}
	return 0.0, nil
}
