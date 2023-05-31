package expression_test

import (
	"testing"

	. "github.com/munnik/gosk/expression"
)

var (
	heights = []interface{}{
		1.230,
		1.315,
		1.390,
		1.460,
		1.555,
		1.640,
		1.705,
		1.775,
		1.840,
		1.905,
		1.975,
		2.040,
		2.100,
		2.170,
		2.240,
		2.310,
		2.380,
		2.450,
		2.520,
		2.590,
		2.660,
	}
	volumes = []interface{}{
		4.000,
		6.000,
		8.000,
		10.000,
		12.000,
		14.000,
		16.000,
		18.000,
		20.000,
		22.000,
		24.000,
		26.000,
		28.000,
		30.000,
		32.000,
		34.000,
		36.000,
		38.000,
		40.000,
		42.000,
		44.000,
	}
)

func TestListToFloatsEmpty(t *testing.T) {
	result, err := ListToFloats([]interface{}{})
	if len(result) != 0 {
		t.Log("Length of result should be 0")
		t.Fail()
	}
	if err != nil {
		t.Log("No error should occur for empty list")
		t.Fail()
	}
}

func TestHeightToVolumeAlmostNoHeight(t *testing.T) {
	vol, err := HeightToVolume(0.029, 1.2, heights, volumes)
	if err != nil {
		t.Log("No error expected")
		t.Fail()
	}
	if vol > 4 {
		t.Log("To much oil measured")
		t.Fail()
	}
}

func TestHeightToVolumeActualHeight(t *testing.T) {
	vol, err := HeightToVolume(0.583925, 1.2, heights, volumes)
	if err != nil {
		t.Log("No error expected")
		t.Fail()
	}
	if vol < 18 {
		t.Logf("Not enough oil measured, %f", vol)
		t.Fail()
	}
	if vol > 20 {
		t.Logf("Too much oil measured, %f", vol)
		t.Fail()
	}
}

func TestCurrentToRatio(t *testing.T) {
	var res float64
	res = CurrentToRatio(4000)
	if res != 0 {
		t.Logf("Expected 0.00 but got %f", res)
		t.Fail()
	}
	res = CurrentToRatio(8000)
	if res != 0.25 {
		t.Logf("Expected 0.25 but got %f", res)
		t.Fail()
	}
	res = CurrentToRatio(12000)
	if res != 0.50 {
		t.Logf("Expected 0.50 but got %f", res)
		t.Fail()
	}
	res = CurrentToRatio(16000)
	if res != 0.75 {
		t.Logf("Expected 0.75 but got %f", res)
		t.Fail()
	}
	res = CurrentToRatio(20000)
	if res != 1.00 {
		t.Logf("Expected 1.00 but got %f", res)
		t.Fail()
	}
}

func TestPressureToHeight(t *testing.T) {
	res := PressureToHeight(4810, 840)
	if res != 0.583925 {
		t.Logf("Expected 0.583925 but got %f", res)
		t.Fail()
	}
}

func TestMilliAmpereToVolume(t *testing.T) {
	ratio := CurrentToRatio(6408 * 1.0)
	pressure := ratio * 30000.0
	height := PressureToHeight(pressure, 840.0)
	res, err := HeightToVolume(height, 1.2, heights, volumes)
	if err != nil {
		t.Logf("Unexpected error %v", err)
		t.Fail()
	}
	if res != 18 {
		t.Logf("Expected 18 but got %f", res)
		t.Fail()
	}
}
