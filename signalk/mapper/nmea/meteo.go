package nmea

import (
	"fmt"

	"github.com/martinlindhe/unit"
)

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

// GetTrueWindDirection retrieves the true wind direction from the sentence
func (s MDA) GetTrueWindDirection() (float64, error) {
	if s.WindDirectionTrue > 0 {
		return (unit.Angle(s.WindDirectionTrue) * unit.Degree).Radians(), nil
	}
	return 0.0, fmt.Errorf("No true wind direction: %s", s)
}

// GetTrueWindDirection retrieves the true wind direction from the sentence
func (s VWR) GetTrueWindDirection() (float64, error) {
	if s.Direction > 0 {
		return (unit.Angle(s.Direction) * unit.Degree).Radians(), nil
	}
	return 0.0, fmt.Errorf("No true wind direction: %s", s)
}

// GetTrueWindDirection retrieves the true wind direction from the sentence
func (s MWV) GetTrueWindDirection() (float64, error) {
	if s.Status != "A" {
		return 0.0, fmt.Errorf("Invalid data: %s", s)
	}
	if s.Angle > 0 && s.Reference == "T" {
		return (unit.Angle(s.Angle) * unit.Degree).Radians(), nil
	}
	return 0.0, fmt.Errorf("No true wind direction: %s", s)
}

// GetRelativeWindDirection retrieves the relative wind direction from the sentence
func (s MWV) GetRelativeWindDirection() (float64, error) {
	if s.Status != "A" {
		return 0.0, fmt.Errorf("Invalid data: %s", s)
	}
	if s.Angle > 0 && s.Reference == "R" {
		return (unit.Angle(s.Angle) * unit.Degree).Radians(), nil
	}
	return 0.0, fmt.Errorf("No relative wind direction: %s", s)
}

// GetMagneticWindDirection retrieves the true wind direction from the sentence
func (s MDA) GetMagneticWindDirection() (float64, error) {
	if s.WindDirectionMagnetic > 0 {
		return (unit.Angle(s.WindDirectionMagnetic) * unit.Degree).Radians(), nil
	}
	return 0.0, fmt.Errorf("No magnetic wind direction: %s", s)
}

// GetWindSpeed retrieves wind speed from the sentence
func (s VWR) GetWindSpeed() (float64, error) {
	if s.WindSpeedInMetersPerSecond > 0 {
		return s.WindSpeedInMetersPerSecond, nil
	}
	if s.WindSpeedInKilometersPerHour > 0 {
		return (unit.Speed(s.WindSpeedInKilometersPerHour) * unit.KilometersPerHour).MetersPerSecond(), nil
	}
	if s.WindSpeedInKnots > 0 {
		return (unit.Speed(s.WindSpeedInKnots) * unit.Knot).MetersPerSecond(), nil
	}
	return 0.0, fmt.Errorf("No wind speed: %s", s)
}

// GetWindSpeed retrieves wind speed from the sentence
func (s MWV) GetWindSpeed() (float64, error) {
	if s.Status != "A" {
		return 0.0, fmt.Errorf("Invalid data: %s", s)
	}
	if s.WindSpeed > 0 {
		if s.WindSpeedUnit == "M" {
			return s.WindSpeed, nil
		}
		if s.WindSpeedUnit == "K" {
			return (unit.Speed(s.WindSpeed) * unit.KilometersPerHour).MetersPerSecond(), nil
		}
		if s.WindSpeedUnit == "N" {
			return (unit.Speed(s.WindSpeed) * unit.Knot).MetersPerSecond(), nil
		}
	}
	return 0.0, fmt.Errorf("No wind speed: %s", s)
}

// GetWindSpeed retrieves wind speed from the sentence
func (s MDA) GetWindSpeed() (float64, error) {
	if s.WindSpeedInMetersPerSecond > 0 {
		return s.WindSpeedInMetersPerSecond, nil
	}
	if s.WindSpeedInKnots > 0 {
		return (unit.Speed(s.WindSpeedInKnots) * unit.Knot).MetersPerSecond(), nil
	}
	return 0.0, fmt.Errorf("No wind speed: %s", s)
}

// GetOutsideTemperature retrieves the outside air temperature from the sentence
func (s MDA) GetOutsideTemperature() (float64, error) {
	// todo: determine no valid value
	return unit.FromCelsius(s.AirTemperature).Kelvin(), nil
}

// GetDewPointTemperature retrieves the dew point temperature from the sentence
func (s MDA) GetDewPointTemperature() (float64, error) {
	// todo: determine no valid value
	return unit.FromCelsius(s.DewPoint).Kelvin(), nil
}

// GetOutsidePressure retrieves the outside pressure from the sentence
func (s MDA) GetOutsidePressure() (float64, error) {
	if s.BarometricPressureInBar > 0 {
		return (unit.Pressure(s.BarometricPressureInBar) * unit.Bar).Pascals(), nil
	}
	if s.BarometricPressureInInchesOfMercury > 0 {
		return (unit.Pressure(s.BarometricPressureInInchesOfMercury) * unit.InchOfMercury).Pascals(), nil
	}
	return 0.0, fmt.Errorf("No outside pressure: %s", s)
}

// GetHumidity retrieves the relative humidity from the sentence
func (s MDA) GetHumidity() (float64, error) {
	return s.RelativeHumidity / 100.0, nil
}
