package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
)

type GGA struct {
	goNMEA.BaseSentence
	Time          goNMEA.Time // Time of fix.
	Latitude      Float64     // Latitude.
	Longitude     Float64     // Longitude.
	FixQuality    string      // Quality of fix.
	NumSatellites Int64       // Number of satellites in use.
	HDOP          Float64     // Horizontal dilution of precision.
	Altitude      Float64     // Altitude.
	Separation    Float64     // Geoidal separation
	DGPSAge       string      // Age of differential GPD data.
	DGPSId        string      // DGPS reference station ID.
}

func init() {
	goNMEA.MustRegisterParser("GGA", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := GGA{
			BaseSentence: s,
			Time:         p.Time(0, "time"),
			FixQuality:   p.EnumString(5, "fix quality", goNMEA.Invalid, goNMEA.GPS, goNMEA.DGPS, goNMEA.PPS, goNMEA.RTK, goNMEA.FRTK, goNMEA.EST),
			DGPSAge:      p.String(12, "dgps age"),
			DGPSId:       p.String(13, "dgps id"),
		}
		if p.Fields[1] != "" && p.Fields[2] != "" {
			result.Latitude = NewFloat64WithValue(p.LatLong(1, 2, "latitude"))
		} else {
			result.Latitude = NewFloat64()
		}
		if p.Fields[3] != "" && p.Fields[4] != "" {
			result.Longitude = NewFloat64WithValue(p.LatLong(4, 5, "longitude"))
		} else {
			result.Longitude = NewFloat64()
		}
		if p.Fields[6] != "" {
			result.NumSatellites = NewInt64WithValue(p.Int64(6, "number of satellites"))
		} else {
			result.NumSatellites = NewInt64()
		}
		if p.Fields[7] != "" {
			result.HDOP = NewFloat64WithValue(p.Float64(7, "hdop"))
		} else {
			result.HDOP = NewFloat64()
		}
		if p.Fields[8] != "" {
			result.Altitude = NewFloat64WithValue(p.Float64(8, "altitude"))
		} else {
			result.Altitude = NewFloat64()
		}
		if p.Fields[10] != "" {
			result.Separation = NewFloat64WithValue(p.Float64(10, "separation"))
		} else {
			result.Separation = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetNumberOfSatellites retrieves the number of satelites from the sentence
func (s GGA) GetNumberOfSatellites() (int64, error) {
	if v, err := s.NumSatellites.GetValue(); err == nil {
		return v, nil
	}
	return 0, fmt.Errorf("value is unavailable")
}

// GetPosition3D retrieves the 3D position from the sentence
func (s GGA) GetPosition3D() (float64, float64, float64, error) {
	if s.FixQuality == goNMEA.GPS || s.FixQuality == goNMEA.DGPS {
		if vLat, err := s.Latitude.GetValue(); err == nil {
			if vLon, err := s.Longitude.GetValue(); err == nil {
				if vAlt, err := s.Altitude.GetValue(); err == nil {
					return vLat, vLon, vAlt, nil
				}
			}
		}
	}
	return 0, 0, 0, fmt.Errorf("value is unavailable")
}

// GetFixQuality retrieves the fix quality from the sentence
func (s GGA) GetFixQuality() (string, error) {
	return s.FixQuality, nil
}
