package nmea

import (
	goNMEA "github.com/adrianmo/go-nmea"
)

type GSA struct {
	goNMEA.BaseSentence
	Mode    string   // The selection mode.
	FixType string   // The fix type.
	SV      []string // List of satellite PRNs used for this fix.
	PDOP    Float64  // Dilution of precision.
	HDOP    Float64  // Horizontal dilution of precision.
	VDOP    Float64  // Vertical dilution of precision.
}

func init() {
	goNMEA.MustRegisterParser("GSA", func(s goNMEA.BaseSentence) (goNMEA.Sentence, error) {
		p := goNMEA.NewParser(s)
		result := GSA{
			BaseSentence: s,
			Mode:         p.EnumString(0, "selection mode", goNMEA.Auto, goNMEA.Manual),
			FixType:      p.EnumString(1, "fix type", goNMEA.FixNone, goNMEA.Fix2D, goNMEA.Fix3D),
		}
		for i := 2; i < 14; i++ {
			if v := p.String(i, "satellite in view"); v != "" {
				result.SV = append(result.SV, v)
			}
		}

		if p.Fields[14] != "" {
			result.PDOP = NewFloat64WithValue(p.Float64(14, "pdop"))
		} else {
			result.PDOP = NewFloat64()
		}
		if p.Fields[15] != "" {
			result.HDOP = NewFloat64WithValue(p.Float64(15, "hdop"))
		} else {
			result.HDOP = NewFloat64()
		}
		if p.Fields[16] != "" {
			result.VDOP = NewFloat64WithValue(p.Float64(16, "vdop"))
		} else {
			result.VDOP = NewFloat64()
		}
		return result, p.Err()
	})
}

// GetNumberOfSatellites retrieves the number of satelites from the sentence
func (s GSA) GetNumberOfSatellites() (int64, error) {
	return int64(len(s.SV)), nil
}

// GetFixType retrieves the fix type from the sentence
func (s GSA) GetFixType() (string, error) {
	return s.FixType, nil
}
