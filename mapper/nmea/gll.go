package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
)

type GLL struct {
	goNMEA.BaseSentence
	Latitude  Float64     // Latitude
	Longitude Float64     // Longitude
	Time      goNMEA.Time // Time Stamp
	Validity  string      // validity - A-valid
}

func init() {
	goNMEA.MustRegisterParser("GLL", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := GLL{
			BaseSentence: s,
			Time:         p.Time(4, "time"),
			Validity:     p.EnumString(5, "validity", goNMEA.ValidGLL, goNMEA.InvalidGLL),
		}
		if p.Fields[0] != "" && p.Fields[1] != "" {
			result.Latitude = NewFloat64(WithValue(p.LatLong(0, 1, "latitude")))
		} else {
			result.Latitude = NewFloat64()
		}
		if p.Fields[2] != "" && p.Fields[3] != "" {
			result.Longitude = NewFloat64(WithValue(p.LatLong(2, 3, "longitude")))
		} else {
			result.Longitude = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetPosition2D retrieves the 2D position from the sentence
func (s GLL) GetPosition2D() (float64, float64, error) {
	if s.Validity == goNMEA.ValidGLL {
		if vLat, err := s.Latitude.GetValue(); err == nil {
			if vLon, err := s.Longitude.GetValue(); err == nil {
				return vLat, vLon, nil
			}
		}
	}
	return 0, 0, fmt.Errorf("value is unavailable")
}
