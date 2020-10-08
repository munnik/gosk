package nmea_test

import (
	"testing"

	goNMEA "github.com/adrianmo/go-nmea"

	"github.com/munnik/gosk/signalk/mapper/nmea"
)

func TestGetPosition2D(t *testing.T) {
	tests := []struct {
		name          string
		s             nmea.Position2D
		wantLongitude float64
		wantLatitude  float64
		wantErr       bool
	}{
		{name: "Empty GLL", s: nmea.GLL{}, wantLongitude: 0.0, wantLatitude: 0.0, wantErr: true},
		{name: "GLL with lon/lat of the Eiffel tower", s: nmea.GLL{Longitude: 2.294481, Latitude: 48.858372}, wantLongitude: 0.0, wantLatitude: 0.0, wantErr: true},
		{name: "GLL with lon/lat of the Eiffel tower and validity set", s: nmea.GLL{Validity: goNMEA.ValidGLL, Longitude: 2.294481, Latitude: 48.858372}, wantLongitude: 2.294481, wantLatitude: 48.858372, wantErr: false},
		{name: "Empty RMC", s: nmea.RMC{}, wantLongitude: 0.0, wantLatitude: 0.0, wantErr: true},
		{name: "RMC with lon/lat of the Eiffel tower", s: nmea.RMC{Longitude: 2.294481, Latitude: 48.858372}, wantLongitude: 0.0, wantLatitude: 0.0, wantErr: true},
		{name: "RMC with lon/lat of the Eiffel tower and validity set", s: nmea.RMC{Validity: goNMEA.ValidRMC, Longitude: 2.294481, Latitude: 48.858372}, wantLongitude: 2.294481, wantLatitude: 48.858372, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotLongitude, gotLatitude, _, err := test.s.GetPosition2D()
			if (err != nil) != test.wantErr {
				t.Errorf("GetPosition2D() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if gotLongitude != test.wantLongitude {
				t.Errorf("GetPosition2D() gotLongitude = %v, want %v", gotLongitude, test.wantLongitude)
			}
			if gotLatitude != test.wantLatitude {
				t.Errorf("GetPosition2D() gotLatitude = %v, want %v", gotLatitude, test.wantLatitude)
			}
		})
	}
}

func TestGetPosition3D(t *testing.T) {
	tests := []struct {
		name          string
		s             nmea.Position3D
		wantLongitude float64
		wantLatitude  float64
		wantAltitude  float64
		wantErr       bool
	}{
		{name: "Empty GGA", s: nmea.GGA{}, wantLongitude: 0.0, wantLatitude: 0.0, wantAltitude: 0.0, wantErr: true},
		{name: "GSA with lon/lat of the Eiffel tower", s: nmea.GGA{Longitude: 2.294481, Latitude: 48.858372, Altitude: 95.0}, wantLongitude: 0.0, wantLatitude: 0.0, wantAltitude: 0.0, wantErr: true},
		{name: "GSA with lon/lat of the Eiffel tower with fix quality set to GPS", s: nmea.GGA{FixQuality: goNMEA.GPS, Longitude: 2.294481, Latitude: 48.858372, Altitude: 95.0}, wantLongitude: 2.294481, wantLatitude: 48.858372, wantAltitude: 95.0, wantErr: false},
		{name: "GSA with lon/lat of the Eiffel tower with fix quality set to DGPS", s: nmea.GGA{FixQuality: goNMEA.DGPS, Longitude: 2.294481, Latitude: 48.858372, Altitude: 95.0}, wantLongitude: 2.294481, wantLatitude: 48.858372, wantAltitude: 95.0, wantErr: false},
		{name: "Empty GNS", s: nmea.GNS{}, wantLongitude: 0.0, wantLatitude: 0.0, wantAltitude: 0.0, wantErr: true},
		{name: "GNS with lon/lat of the Eiffel tower", s: nmea.GNS{Longitude: 2.294481, Latitude: 48.858372, Altitude: 95.0}, wantLongitude: 0.0, wantLatitude: 0.0, wantAltitude: 0.0, wantErr: true},
		{name: "GNS with lon/lat of the Eiffel tower with mode set to DifferentialGNS", s: nmea.GNS{Mode: []string{goNMEA.DifferentialGNS}, Longitude: 2.294481, Latitude: 48.858372, Altitude: 95.0}, wantLongitude: 2.294481, wantLatitude: 48.858372, wantAltitude: 95.0, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotLongitude, gotLatitude, gotAltitude, _, err := test.s.GetPosition3D()
			if (err != nil) != test.wantErr {
				t.Errorf("GetPosition3D() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if gotLongitude != test.wantLongitude {
				t.Errorf("GetPosition3D() gotLongitude = %v, want %v", gotLongitude, test.wantLongitude)
			}
			if gotLatitude != test.wantLatitude {
				t.Errorf("GetPosition3D() gotLatitude = %v, want %v", gotLatitude, test.wantLatitude)
			}
			if gotAltitude != test.wantAltitude {
				t.Errorf("GetPosition3D() gotAltitude = %v, want %v", gotAltitude, test.wantAltitude)
			}
		})
	}
}
