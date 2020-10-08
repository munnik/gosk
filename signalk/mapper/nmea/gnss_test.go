package nmea_test

import (
	"testing"

	"github.com/munnik/gosk/signalk/mapper/nmea"
)

func TestGetFixQuality(t *testing.T) {
	tests := []struct {
		name    string
		s       nmea.FixQuality
		want    string
		wantErr bool
	}{
		{name: "Empty GGA", s: nmea.GGA{}, want: "", wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetFixQuality()
			if (err != nil) != test.wantErr {
				t.Errorf("GetFixQuality() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("GetFixQuality() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGetFixType(t *testing.T) {
	tests := []struct {
		name    string
		s       nmea.FixType
		want    string
		wantErr bool
	}{
		{name: "Empty GSA", s: nmea.GSA{}, want: "", wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetFixType()
			if (err != nil) != test.wantErr {
				t.Errorf("GetFixType() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("GetFixType() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGetNumberOfSatelites(t *testing.T) {
	tests := []struct {
		name    string
		s       nmea.NumberOfSatelites
		want    int64
		wantErr bool
	}{
		{name: "Empty GGA", s: nmea.GGA{}, want: 0, wantErr: false},
		{name: "Empty GSA", s: nmea.GSA{}, want: 0, wantErr: false},
		{name: "Empty GSV", s: nmea.GSV{}, want: 0, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetNumberOfSatelites()
			if (err != nil) != test.wantErr {
				t.Errorf("GetNumberOfSatelites() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("GetNumberOfSatelites() = %v, want %v", got, test.want)
			}
		})
	}
}
