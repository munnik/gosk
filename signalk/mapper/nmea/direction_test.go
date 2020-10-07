package nmea_test

import (
	"math"
	"testing"

	goNMEA "github.com/adrianmo/go-nmea"

	"github.com/munnik/gosk/signalk/mapper/nmea"
)

func TestGetMagneticCourseOverGround(t *testing.T) {
	var tests = []struct {
		name    string
		s       nmea.MagneticCourseOverGround
		want    float64
		wantErr bool
	}{
		{name: "Empty VTG", s: nmea.VTG{}, want: 0.0, wantErr: false},
		{name: "VTG with only MagneticTrack", s: nmea.VTG{MagneticTrack: 270}, want: 1.5 * math.Pi, wantErr: false},
		{name: "VTG with only TrueTrack", s: nmea.VTG{TrueTrack: 180}, want: 0.0, wantErr: false},
		{name: "VTG with both MagneticTrack and TrueTrack", s: nmea.VTG{MagneticTrack: 270, TrueTrack: 180}, want: 1.5 * math.Pi, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetmagneticCourseOverGround()
			if (err != nil) != test.wantErr {
				t.Errorf("GetmagneticCourseOverGround() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("GetmagneticCourseOverGround() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGetMagneticHeading(t *testing.T) {
	var tests = []struct {
		name    string
		s       nmea.MagneticHeading
		want    float64
		wantErr bool
	}{
		{name: "Empty HDT", s: nmea.HDT{}, want: 0.0, wantErr: false},
		{name: "HDT with heading true/magnetic set to magnetic", s: nmea.HDT{True: false}, want: 0.0, wantErr: false},
		{name: "HDT with heading true/magnetic set to true", s: nmea.HDT{True: true}, want: 0.0, wantErr: true},
		{name: "HDT with heading set to 180", s: nmea.HDT{Heading: 180}, want: math.Pi, wantErr: false},
		{name: "HDT with heading set to 180 and heading true/magnetic set to magnetic", s: nmea.HDT{Heading: 270}, want: 1.5 * math.Pi, wantErr: false},
		{name: "HDT with heading set to 180 and heading true/magnetic set to true", s: nmea.HDT{Heading: 180, True: true}, want: 0.0, wantErr: true},
		{name: "Empty VHW", s: nmea.VHW{}, want: 0.0, wantErr: false},
		{name: "VHW with magnetic heading set", s: nmea.VHW{MagneticHeading: 270}, want: 1.5 * math.Pi, wantErr: false},
		{name: "VHW with true heading set", s: nmea.VHW{TrueHeading: 180}, want: 0.0, wantErr: false},
		{name: "VHW with both magnetic heading and true heading set", s: nmea.VHW{MagneticHeading: 270, TrueHeading: 180}, want: 1.5 * math.Pi, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetMagneticHeading()
			if (err != nil) != test.wantErr {
				t.Errorf("GetMagneticHeading() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("GetMagneticHeading() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGetMagneticVariation(t *testing.T) {
	var tests = []struct {
		name    string
		s       nmea.MagneticVariation
		want    float64
		wantErr bool
	}{
		{name: "Empty RMC", s: nmea.RMC{}, want: 0.0, wantErr: true},
		{name: "RMC with only variation set", s: nmea.RMC{Variation: 180}, want: 0.0, wantErr: true},
		{name: "RMC with variation set and validity set to true", s: nmea.RMC{Variation: 270, Validity: goNMEA.ValidRMC}, want: 1.5 * math.Pi, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetMagneticVariation()
			if (err != nil) != test.wantErr {
				t.Errorf("GetMagneticVariation() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("GetMagneticVariation() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGetTrueCourseOverGround(t *testing.T) {
	var tests = []struct {
		name    string
		s       nmea.TrueCourseOverGround
		want    float64
		wantErr bool
	}{
		{name: "Empty RMC", s: nmea.RMC{}, want: 0.0, wantErr: true},
		{name: "RMC with only course set", s: nmea.RMC{Course: 180}, want: 0.0, wantErr: true},
		{name: "RMC with course set and validity set to true", s: nmea.RMC{Course: 180, Validity: goNMEA.ValidRMC}, want: math.Pi, wantErr: false},
		{name: "Empty VTG", s: nmea.VTG{}, want: 0.0, wantErr: false},
		{name: "VTG with magnetic track set", s: nmea.VTG{MagneticTrack: 270}, want: 0.0, wantErr: false},
		{name: "VTG with true track set", s: nmea.VTG{TrueTrack: 180}, want: math.Pi, wantErr: false},
		{name: "VTG with both magnetic track and true track set", s: nmea.VTG{MagneticTrack: 270, TrueTrack: 180}, want: math.Pi, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetTrueCourseOverGround()
			if (err != nil) != test.wantErr {
				t.Errorf("GetTrueCourseOverGround() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("GetTrueCourseOverGround() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGetTrueHeading(t *testing.T) {
	var tests = []struct {
		name    string
		s       nmea.TrueHeading
		want    float64
		wantErr bool
	}{
		{name: "Empty HDT", s: nmea.HDT{}, want: 0.0, wantErr: true},
		{name: "HDT with heading set to magnetic", s: nmea.HDT{True: false}, want: 0.0, wantErr: true},
		{name: "HDT with heading set to true", s: nmea.HDT{True: true}, want: 0.0, wantErr: false},
		{name: "HDT with heading set", s: nmea.HDT{Heading: 180}, want: 0.0, wantErr: true},
		{name: "HDT with heading set and heading set to true", s: nmea.HDT{Heading: 180, True: true}, want: math.Pi, wantErr: false},
		{name: "Empty VHW", s: nmea.VHW{}, want: 0.0, wantErr: false},
		{name: "VHW with magnetic heading set", s: nmea.VHW{MagneticHeading: 270}, want: 0.0, wantErr: false},
		{name: "VHW with true heading set", s: nmea.VHW{TrueHeading: 180}, want: math.Pi, wantErr: false},
		{name: "Empty THS", s: nmea.THS{}, want: 0.0, wantErr: true},
		{name: "THS with only heading set", s: nmea.THS{Heading: 180}, want: 0.0, wantErr: true},
		{name: "THS with heading set and status set to autonomous", s: nmea.THS{Heading: 180, Status: goNMEA.AutonomousTHS}, want: math.Pi, wantErr: false},
		{name: "THS with heading set and status set to autonomous", s: nmea.THS{Heading: 180, Status: goNMEA.EstimatedTHS}, want: 0.0, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetTrueHeading()
			if (err != nil) != test.wantErr {
				t.Errorf("GetTrueHeading() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("GetTrueHeading() = %v, want %v", got, test.want)
			}
		})
	}
}
