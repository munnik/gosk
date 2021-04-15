package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
)

type GSV struct {
	goNMEA.BaseSentence
	TotalMessages   Int64     // Total number of messages of this type in this cycle
	MessageNumber   Int64     // Message number
	NumberSVsInView Int64     // Total number of SVs in view
	Info            []GSVInfo // visible satellite info (0-4 of these)
}

// GSVInfo represents information about a visible satellite
type GSVInfo struct {
	SVPRNNumber Int64 // SV PRN number, pseudo-random noise or gold code
	Elevation   Int64 // Elevation in degrees, 90 maximum
	Azimuth     Int64 // Azimuth, degrees from true north, 000 to 359
	SNR         Int64 // SNR, 00-99 dB (null when not tracking)
}

func init() {
	goNMEA.MustRegisterParser("GSV", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := GSV{
			BaseSentence: s,
		}
		if p.Fields[0] != "" {
			result.TotalMessages = NewInt64WithValue(p.Int64(0, "total number of messages"))
		} else {
			result.TotalMessages = NewInt64()
		}
		if p.Fields[1] != "" {
			result.MessageNumber = NewInt64WithValue(p.Int64(1, "message number"))
		} else {
			result.MessageNumber = NewInt64()
		}
		if p.Fields[2] != "" {
			result.NumberSVsInView = NewInt64WithValue(p.Int64(2, "number of SVs in view"))
		} else {
			result.NumberSVsInView = NewInt64()
		}
		for i := 0; i < 4; i++ {
			if 5*i+4 > len(result.Fields) {
				break
			}
			result.Info = append(result.Info, GSVInfo{
				SVPRNNumber: NewInt64WithValue(p.Int64(3+i*4, "SV prn number")),
				Elevation:   NewInt64WithValue(p.Int64(4+i*4, "elevation")),
				Azimuth:     NewInt64WithValue(p.Int64(5+i*4, "azimuth")),
				SNR:         NewInt64WithValue(p.Int64(6+i*4, "SNR")),
			})
		}
		return result, p.Err()
	})
}

// GetNumberOfSatelites retrieves the number of satelites from the sentence
func (s GSV) GetNumberOfSatellites() (int64, error) {
	if v, err := s.NumberSVsInView.GetValue(); err == nil {
		return v, nil
	}
	return 0, fmt.Errorf("value is unavailable")
}
