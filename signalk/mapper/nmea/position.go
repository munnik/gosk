package nmea

import (
	"errors"
	"fmt"

	goAIS "github.com/BertoldVdb/go-ais"
	goNMEA "github.com/adrianmo/go-nmea"
)

// Position2D retrieves the 2D position from the sentence
type Position2D interface {
	GetPosition2D() (float64, float64, error)
}

// Position3D retrieves the 3D position from the sentence
type Position3D interface {
	GetPosition3D() (float64, float64, float64, error)
}

// GetPosition2D retrieves the 2D position from the sentence
func (s GLL) GetPosition2D() (float64, float64, error) {
	if s.Validity != goNMEA.ValidGLL {
		return 0.0, 0.0, fmt.Errorf("The invalid flag is set to %s in the sentence: %s", s.Validity, s)
	}
	return s.Longitude, s.Latitude, nil
}

// GetPosition2D retrieves the 2D position from the sentence
func (s RMC) GetPosition2D() (float64, float64, error) {
	if s.Validity != goNMEA.ValidRMC {
		return 0.0, 0.0, fmt.Errorf("The invalid flag is set to %s in the sentence: %s", s.Validity, s)
	}
	return s.Longitude, s.Latitude, nil
}

// GetPosition2D retrieves the 2D position from the sentence
func (s VDMVDO) GetPosition2D() (float64, float64, error) {
	if positionReport, ok := s.Packet.(goAIS.PositionReport); ok && positionReport.Valid {
		return float64(positionReport.Longitude), float64(positionReport.Latitude), nil
	}
	return 0.0, 0.0, errors.New("Not a position report or invalid position report")
}

// GetPosition3D retrieves the 3D position from the sentence
func (s GGA) GetPosition3D() (float64, float64, float64, error) {
	if s.FixQuality != goNMEA.GPS && s.FixQuality != goNMEA.DGPS {
		return 0.0, 0.0, 0.0, fmt.Errorf("The fix quality is set to %s in the sentence: %s", s.FixQuality, s)

	}
	return s.Longitude, s.Latitude, s.Altitude, nil
}

// GetPosition3D retrieves the 3D position from the sentence
func (s GNS) GetPosition3D() (float64, float64, float64, error) {
	for _, m := range s.Mode {
		if m == goNMEA.AutonomousGNS || m == goNMEA.DifferentialGNS || m == goNMEA.PreciseGNS || m == goNMEA.RealTimeKinematicGNS || m == goNMEA.FloatRTKGNS {
			return s.Longitude, s.Latitude, s.Altitude, nil
		}
	}
	return 0.0, 0.0, 0.0, fmt.Errorf("These non acceptable modes %v are found in the sentence: %s", s.Mode, s)
}
