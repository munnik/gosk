package nmea

import (
	"fmt"

	goAIS "github.com/BertoldVdb/go-ais"
	"github.com/BertoldVdb/go-ais/aisnmea"

	goNMEA "github.com/adrianmo/go-nmea"
)

type (
	VDMVDO struct {
		goNMEA.BaseSentence
		goAIS.Packet
	}
)

// Float64 can contain nil values
type Float64 struct {
	value float64
	isSet bool
}

// Int64 can contain nil values
type Int64 struct {
	value int64
	isSet bool
}

// MagneticCourseOverGround retrieves the magnetic course over ground from the sentence
type MagneticCourseOverGround interface {
	GetmagneticCourseOverGround() (float64, error)
}

// MagneticHeading retrieves the magnetic heading from the sentence
type MagneticHeading interface {
	GetMagneticHeading() (float64, error)
}

// MagneticVariation retrieves the magnetic variation from the sentence
type MagneticVariation interface {
	GetMagneticVariation() (float64, error)
}

// RateOfTurn retrieves the rate of turn from the sentence
type RateOfTurn interface {
	GetRateOfTurn() (float64, error)
}

// TrueCourseOverGround retrieves the true course over ground from the sentence
type TrueCourseOverGround interface {
	GetTrueCourseOverGround() (float64, error)
}

// TrueHeading retrieves the true heading from the sentence
type TrueHeading interface {
	GetTrueHeading() (float64, error)
}

// FixQuality retrieves the fix quality from the sentence
type FixQuality interface {
	GetFixQuality() (string, error)
}

// FixType retrieves the fix type from the sentence
type FixType interface {
	GetFixType() (string, error)
}

// NumberOfSatelites retrieves the number of satelites from the sentence
type NumberOfSatelites interface {
	GetNumberOfSatelites() (int64, error)
}

// Position2D retrieves the 2D position from the sentence
type Position2D interface {
	GetPosition2D() (float64, float64, error)
}

// Position3D retrieves the 3D position from the sentence
type Position3D interface {
	GetPosition3D() (float64, float64, float64, error)
}

// SpeedOverGround retrieves the speed over ground from the sentence
type SpeedOverGround interface {
	GetSpeedOverGround() (float64, error)
}

// SpeedThroughWater retrieves the speed through water from the sentence
type SpeedThroughWater interface {
	GetSpeedThroughWater() (float64, error)
}

// DepthBelowSurface retrieves the depth below surface from the sentence
type DepthBelowSurface interface {
	GetDepthBelowSurface() (float64, error)
}

// DepthBelowSurface retrieves the depth below surface from the sentence
type DepthBelowKeel interface {
	GetDepthBelowKeel() (float64, error)
}

// DepthBelowTransducer retrieves the depth below the transducer from the sentence
type DepthBelowTransducer interface {
	GetDepthBelowTransducer() (float64, error)
}

// WaterTemperature retrieves the water temperature from the sentence
type WaterTemperature interface {
	GetWaterTemperature() (float64, error)
}

// TrueWindDirection retrieves the true wind direction from the sentence
type TrueWindDirection interface {
	GetTrueWindDirection() (float64, error)
}

// MagneticWindDirection retrieves the magnetic wind direction from the sentence
type MagneticWindDirection interface {
	GetMagneticWindDirection() (float64, error)
}

// RelativeWindDirection retrieves the relative wind direction from the sentence
type RelativeWindDirection interface {
	GetRelativeWindDirection() (float64, error)
}

// WindSpeed retrieves the wind speed from the sentence
type WindSpeed interface {
	GetWindSpeed() (float64, error)
}

// OutsideTemperature retrieves the outside air temperature from the sentence
type OutsideTemperature interface {
	GetOutsideTemperature() (float64, error)
}

// DewPointTemperature retrieves the dew point temperature from the sentence
type DewPointTemperature interface {
	GetDewPointTemperature() (float64, error)
}

// Humidity retrieves the relative humidity from the sentence
type Humidity interface {
	GetHumidity() (float64, error)
}

var nmeaCodec *aisnmea.NMEACodec
var aisCodec *goAIS.Codec

func init() {
	aisCodec = goAIS.CodecNew(false, false)
	aisCodec.DropSpace = true
	nmeaCodec = aisnmea.NMEACodecNew(aisCodec)
}

// Parse is a wrapper around the original Parse function, it returns types defined in this package that implement the interfaces in this package
func Parse(raw string) (goNMEA.Sentence, error) {
	sentence, err := goNMEA.Parse(raw)
	if err != nil {
		return nil, err
	}
	switch sentence.DataType() {
	case goNMEA.TypeVDM, goNMEA.TypeVDO:
		return ParseVDMVDO(sentence), nil
	}

	return sentence, nil
}

func NewFloat64() Float64 {
	return Float64{}
}

func NewFloat64WithValue(v float64) Float64 {
	return Float64{
		value: v,
		isSet: true,
	}
}

func (v Float64) GetValue() (float64, error) {
	if v.isSet {
		return v.value, nil
	}
	return 0, fmt.Errorf("the value is nil")
}

func NewInt64() Int64 {
	return Int64{}
}

func NewInt64WithValue(v int64) Int64 {
	return Int64{
		value: v,
		isSet: true,
	}
}

func (v Int64) GetValue() (int64, error) {
	if v.isSet {
		return v.value, nil
	}
	return 0, fmt.Errorf("the value is nil")
}
