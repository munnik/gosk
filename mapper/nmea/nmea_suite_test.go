package nmea_test

import (
	"fmt"
	"math"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

const (
	MagneticDirectionRadians = float64(4.0631264986427995)
	MagneticDirectionDegrees = float64(232.8)

	TrueDirectionRadians = float64(4.094542425178697)
	TrueDirectionDegrees = float64(234.6)

	RelativeDirectionRadians = float64(-0.65973445725)
	RelativeDirectionDegrees = float64(-37.8)

	MagneticVariationRadians = float64(0.04014257)
	MagneticVariationDegrees = float64(2.3)

	SpeedOverGroundMPS   = float64(3.7222252000000005)
	SpeedOverGroundKPH   = float64(13.4)
	SpeedOverGroundKnots = float64(7.235418)

	SpeedThroughWaterMPS   = float64(3.1388889)
	SpeedThroughWaterKPH   = float64(11.3)
	SpeedThroughWaterKnots = float64(6.1015119)

	Longitude = float64(2.294481)
	Latitude  = float64(48.858372)
	Altitude  = float64(2.3)

	Satellites = int64(11)

	DepthBelowSurfaceMeters  = float64(3.9)
	DepthBelowSurfaceFeet    = float64(12.79528)
	DepthBelowSurfaceFathoms = float64(2.1325459)

	DepthTransducerMeters   = float64(1.8)
	DepthTransducerFeet     = float64(5.905512)
	DepthTransducerFanthoms = float64(0.98425197)

	DepthKeelMeters   = float64(1.95)
	DepthKeelFeet     = float64(6.397638)
	DepthKeelFanthoms = float64(1.0662730)

	PressurePascal          = 101600
	PressureBar             = 1.016
	PressureInchesOfMercury = 30.0026

	AirTemperatureKelvin  = 290.65
	AirTemperatureCelcius = 17.5

	WaterTemperatureKelvin  = 282.05
	WaterTemperatureCelcius = 8.9

	DewPointKelvin  = 280.35
	DewPointCelcius = 7.2

	RelativeHumidityPercentage = 55.6
	RelativeHumidityRatio      = 0.556
)

func TestNmea(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Nmea Suite")
}

// Float64Matcher is used to match two float64s and make sure the are 'close enough'
type Float64Matcher struct {
	Expected float64
	Delta    float64
}

func Float64Equal(expected float64, delta float64) types.GomegaMatcher {
	return &Float64Matcher{
		Expected: expected,
		Delta:    delta,
	}
}

func (matcher *Float64Matcher) Match(actual interface{}) (bool, error) {
	actualFloat, ok := actual.(float64)
	if !ok {
		return false, fmt.Errorf("matcher expects a float64")
	}

	return math.Abs(matcher.Expected-actualFloat) <= matcher.Delta, nil
}

func (matcher *Float64Matcher) FailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected\n\t%#v\nto be close enough (delta less than or equal to %f) to\n\t%f", actual, matcher.Delta, matcher.Expected)
}

func (matcher *Float64Matcher) NegatedFailureMessage(actual interface{}) string {
	return fmt.Sprintf("Expected\n\t%#v\nnot to be close enough (delta less than or equal to %f) to\n\t%f", actual, matcher.Delta, matcher.Expected)
}
