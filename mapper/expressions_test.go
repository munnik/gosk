package mapper_test

import (
	"math"
	"testing"

	. "github.com/munnik/gosk/mapper"
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

// closeTo compares two floats with a tolerance, the results of these
// conversions are not exactly representable in binary floating point.
func closeTo(got float64, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

func TestPressureToHeight(t *testing.T) {
	// 4810 Pa of diesel (840 kg/m3) stands 4810 / (840 * 9.8) m high
	res := PressureToHeight(4810, 840)
	if !closeTo(res, 0.5843051506316812) {
		t.Logf("Expected 0.5843051506316812 but got %f", res)
		t.Fail()
	}
}

func TestBetween(t *testing.T) {
	between := func(value, min, max interface{}) bool {
		result, err := Between(value, min, max)
		if err != nil {
			t.Logf("Unexpected error %v", err)
			t.Fail()
		}
		return result
	}

	if !between(5, 0, 10) {
		t.Log("Expected 5 to be between 0 and 10")
		t.Fail()
	}
	if !between(0, 0, 10) {
		t.Log("Expected the lower bound to be inclusive")
		t.Fail()
	}
	if !between(10, 0, 10) {
		t.Log("Expected the upper bound to be inclusive")
		t.Fail()
	}
	if between(-1, 0, 10) {
		t.Log("Expected -1 to not be between 0 and 10")
		t.Fail()
	}
	if between(11, 0, 10) {
		t.Log("Expected 11 to not be between 0 and 10")
		t.Fail()
	}
	if !between(5.5, 0, 10) {
		t.Log("Expected a float64 value to work")
		t.Fail()
	}
	if !between(uint16(5), int64(0), float32(10)) {
		t.Log("Expected mixed numeric types to work")
		t.Fail()
	}
	if _, err := Between("not a number", 0, 10); err == nil {
		t.Log("Expected an error for a non-numeric value")
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
	// 6408 uA is a ratio of 0.1505, so 4515 Pa, so a height of 0.548469 m.
	// With the sensor 1.2 m above the bottom that is 1.748469 m, which sits
	// between the 1.705 m / 16 m3 and 1.775 m / 18 m3 rows of the table.
	if !closeTo(res, 17.241982507288625) {
		t.Logf("Expected 17.241982507288625 but got %f", res)
		t.Fail()
	}
}

func TestRandom(t *testing.T) {
	seen := make(map[float64]bool)
	for i := 0; i < 1000; i++ {
		res := Random()
		if res < 0.0 || res >= 1.0 {
			t.Logf("Expected a value in [0.0, 1.0) but got %f", res)
			t.Fail()
		}
		seen[res] = true
	}
	if len(seen) < 900 {
		t.Logf("Expected mostly different values but only got %d different ones out of 1000", len(seen))
		t.Fail()
	}
}
