package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
)

type GNS struct {
	goNMEA.BaseSentence
	Time       goNMEA.Time
	Latitude   Float64
	Longitude  Float64
	Mode       []string
	SVs        Int64
	HDOP       Float64
	Altitude   Float64
	Separation Float64
	Age        Float64
	Station    Int64
}

func init() {
	goNMEA.MustRegisterParser("GNS", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := GNS{
			BaseSentence: s,
			Time:         p.Time(0, "time"),
			Mode:         p.EnumChars(5, "mode", goNMEA.NoFixGNS, goNMEA.AutonomousGNS, goNMEA.DifferentialGNS, goNMEA.PreciseGNS, goNMEA.RealTimeKinematicGNS, goNMEA.FloatRTKGNS, goNMEA.EstimatedGNS, goNMEA.ManualGNS, goNMEA.SimulatorGNS),
		}
		if p.Fields[1] != "" && p.Fields[2] != "" {
			result.Latitude = NewFloat64WithValue(p.LatLong(1, 2, "latitude"))
		} else {
			result.Latitude = NewFloat64()
		}
		if p.Fields[3] != "" && p.Fields[4] != "" {
			result.Longitude = NewFloat64WithValue(p.LatLong(3, 4, "longitude"))
		} else {
			result.Longitude = NewFloat64()
		}
		if p.Fields[2] != "" {
			result.SVs = NewInt64WithValue(p.Int64(6, "SVs"))
		} else {
			result.SVs = NewInt64()
		}
		if p.Fields[7] != "" {
			result.HDOP = NewFloat64WithValue(p.Float64(7, "HDOP"))
		} else {
			result.HDOP = NewFloat64()
		}
		if p.Fields[8] != "" {
			result.Altitude = NewFloat64WithValue(p.Float64(8, "altitude"))
		} else {
			result.Altitude = NewFloat64()
		}
		if p.Fields[9] != "" {
			result.Separation = NewFloat64WithValue(p.Float64(9, "separation"))
		} else {
			result.Separation = NewFloat64()
		}
		if p.Fields[10] != "" {
			result.Age = NewFloat64WithValue(p.Float64(10, "age"))
		} else {
			result.Age = NewFloat64()
		}
		if p.Fields[11] != "" {
			result.Station = NewInt64WithValue(p.Int64(11, "station"))
		} else {
			result.Station = NewInt64()
		}
		return result, p.Err()
	})
}

// GetPosition3D retrieves the 3D position from the sentence
func (s GNS) GetPosition3D() (float64, float64, float64, error) {
	validModi := map[string]interface{}{
		goNMEA.AutonomousGNS:        nil,
		goNMEA.DifferentialGNS:      nil,
		goNMEA.PreciseGNS:           nil,
		goNMEA.RealTimeKinematicGNS: nil,
		goNMEA.FloatRTKGNS:          nil,
		goNMEA.EstimatedGNS:         nil,
		goNMEA.ManualGNS:            nil,
		goNMEA.SimulatorGNS:         nil,
	}
	for _, m := range s.Mode {
		if _, ok := validModi[m]; ok && s.Longitude.isSet && s.Latitude.isSet && s.Altitude.isSet {
			if vLat, err := s.Latitude.GetValue(); err == nil {
				if vLon, err := s.Longitude.GetValue(); err == nil {
					if vAlt, err := s.Altitude.GetValue(); err == nil {
						return vLat, vLon, vAlt, nil
					}
				}
			}
		}
	}
	return 0, 0, 0, fmt.Errorf("value is unavailable")
}
