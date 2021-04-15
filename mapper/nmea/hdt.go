package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

type HDT struct {
	goNMEA.BaseSentence
	Heading Float64 // Heading in degrees
	True    bool    // Heading is relative to true north
}

func init() {
	goNMEA.MustRegisterParser("HDT", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := HDT{
			BaseSentence: s,
			True:         p.EnumString(1, "true", "T") == "T",
		}
		if p.Fields[0] != "" && result.True {
			result.Heading = NewFloat64WithValue(p.Float64(0, "heading"))
		} else {
			result.Heading = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetTrueHeading retrieves the true heading from the sentence
func (s HDT) GetTrueHeading() (float64, error) {
	if v, err := s.Heading.GetValue(); err == nil && s.True {
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}
