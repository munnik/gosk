package nmea_test

import (
	"math"
	"testing"

	"github.com/munnik/gosk/signalk/parser/nmea"
)

func TestGetSpeedOverGround(t *testing.T) {
	tests := []struct {
		name    string
		s       nmea.SpeedOverGround
		want    float64
		wantErr bool
	}{
		{name: "Empty RMC", s: nmea.RMC{}, want: 0.0, wantErr: true},
		{name: "Empty VTG", s: nmea.RMC{}, want: 0.0, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetSpeedOverGround()
			if (err != nil) != test.wantErr {
				t.Errorf("GetSpeedOverGround() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if math.Abs(got-test.want) > 0.0001 {
				t.Errorf("GetSpeedOverGround() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGetSpeedThroughWater(t *testing.T) {
	tests := []struct {
		name    string
		s       nmea.SpeedThroughWater
		want    float64
		wantErr bool
	}{
		{name: "Empty VHW", s: nmea.VHW{}, want: 0.0, wantErr: false},
		{name: "VHW with speed through water in kmh", s: nmea.VHW{SpeedThroughWaterKPH: 12}, want: 3.333336, wantErr: false},
		{name: "VHW with speed through water in knots", s: nmea.VHW{SpeedThroughWaterKnots: 7}, want: 3.601108, wantErr: false},
		{name: "VHW with both speed through water in kmh and speed through water in knots", s: nmea.VHW{SpeedThroughWaterKPH: 12, SpeedThroughWaterKnots: 7}, want: 3.333336, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetSpeedThroughWater()
			if (err != nil) != test.wantErr {
				t.Errorf("GetSpeedThroughWater() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if math.Abs(got-test.want) > 0.0001 {
				t.Errorf("GetSpeedThroughWater() = %v, want %v", got, test.want)
			}
		})
	}
}
