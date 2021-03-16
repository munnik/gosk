package nmea0183

import (
	"math"
	"testing"
)

func TestGetDepthBelowSurface(t *testing.T) {
	var tests = []struct {
		name    string
		s       DepthBelowSurface
		want    float64
		wantErr bool
	}{
		{name: "Empty DBS", s: DBS{}, want: 0.0, wantErr: true},
		{name: "DBS with meters", s: DBS{DepthMeters: 3.4}, want: 3.4, wantErr: false},
		{name: "DBS with meters and fathoms", s: DBS{DepthMeters: 3.4, DepthFathoms: 5}, want: 3.4, wantErr: false},
		{name: "DBS with fathoms", s: DBS{DepthFathoms: 1.8591425}, want: 3.4, wantErr: false},
		{name: "DBS with feet", s: DBS{DepthFeet: 11.15486}, want: 3.4, wantErr: false},
		{name: "DBS with meters and feet", s: DBS{DepthMeters: 3.4, DepthFeet: 12}, want: 3.4, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetDepthBelowSurface()
			if (err != nil) != test.wantErr {
				t.Errorf("GetDepthBelowSurface() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if math.Abs(got-test.want) > 0.0001 {
				t.Errorf("GetDepthBelowSurface() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestGetDepthBelowTransducer(t *testing.T) {
	var tests = []struct {
		name    string
		s       DepthBelowTransducer
		want    float64
		wantErr bool
	}{
		{name: "Empty DBT", s: DBT{}, want: 0.0, wantErr: true},
		{name: "DBT with meters", s: DBT{DepthMeters: 3.4}, want: 3.4, wantErr: false},
		{name: "DBT with meters and fathoms", s: DBT{DepthMeters: 3.4, DepthFathoms: 5}, want: 3.4, wantErr: false},
		{name: "DBT with fathoms", s: DBT{DepthFathoms: 1.8591425}, want: 3.4, wantErr: false},
		{name: "DBT with feet", s: DBT{DepthFeet: 11.15486}, want: 3.4, wantErr: false},
		{name: "DBT with meters and feet", s: DBT{DepthMeters: 3.4, DepthFeet: 12}, want: 3.4, wantErr: false},
		{name: "Empty DPT", s: DPT{}, want: 0.0, wantErr: false},
		{name: "DPT with depth", s: DPT{Depth: 3.4}, want: 3.4, wantErr: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.s.GetDepthBelowTransducer()
			if (err != nil) != test.wantErr {
				t.Errorf("GetDepthBelowTransducer() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if math.Abs(got-test.want) > 0.0001 {
				t.Errorf("GetDepthBelowTransducer() = %v, want %v", got, test.want)
			}
		})
	}
}
