package nmea

import (
	"fmt"

	goNMEA "github.com/adrianmo/go-nmea"
	"github.com/martinlindhe/unit"
)

type THS struct {
	goNMEA.BaseSentence
	Heading Float64 // Heading in degrees
	Status  string  // Heading status
}

func init() {
	goNMEA.MustRegisterParser("THS", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := THS{
			BaseSentence: s,
			Status:       p.EnumString(1, "status", goNMEA.AutonomousTHS, goNMEA.EstimatedTHS, goNMEA.ManualTHS, goNMEA.SimulatorTHS, goNMEA.InvalidTHS),
		}
		if p.Fields[0] != "" && result.Status != goNMEA.InvalidTHS {
			result.Heading = NewFloat64WithValue(p.Float64(0, "heading"))
		} else {
			result.Heading = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetTrueHeading retrieves the true heading from the sentence
func (s THS) GetTrueHeading() (float64, error) {
	if v, err := s.Heading.GetValue(); err == nil && s.Status != goNMEA.InvalidTHS {
		return (unit.Angle(v) * unit.Degree).Radians(), nil
	}
	return 0, fmt.Errorf("value is unavailable")
}
