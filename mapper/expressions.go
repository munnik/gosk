package mapper

import "fmt"

type ExpressionEnvironment map[string]interface{}

func NewExpressionEnvironment() ExpressionEnvironment {
	return ExpressionEnvironment{
		"currentToRatio":   currentToRatio,
		"pressureToHeight": pressureToHeight,
		"heightToVolume":   heightToVolume,
	}
}

// Returns the 4-20mA input signal to a ratio, 4000uA => 0.0, 8000uA => 0.25, 12000uA => 0.5, 16000uA => 0.75, 20000uA => 1.0
// current is in uA (1000000uA is 1A)
// return value is a ratio (0.0 .. 1.0)
func currentToRatio(current float64) float64 {
	return (current - 4000) / 16000
}

// Converts a pressure and density to a height
// pressure is in Pa (1 Bar is 100000 Pascal)
// density is in kg/m3 (typical value for diesel is 840)
// return value is in m
func pressureToHeight(pressure float64, density float64) float64 {
	G := 9.8 // acceleration due to gravity
	return pressure / (density * G)
}

// Returns the heightToVolume corresponding to the measured height. This function is used when a pressure sensor is used in a tank.
// height is in m
// sensorOffset is in m (positive means that the sensor is placed above the bottom of the tank, negative value means that the sensor is place below the tank)
// heights is in m, list of heights with corresponding volumes
// volumes is in m3, list of volumes with corresponding heights
// return value is in m3
func heightToVolume(height float64, sensorOffset float64, heights []interface{}, volumes []interface{}) (result float64, err error) {
	if len(heights) != len(volumes) {
		err = fmt.Errorf("The list of heights should have the same length as the list of volumes, the lengths are %d and %d", len(heights), len(volumes))
		return
	}

	heightFloats := make([]float64, len(heights))
	volumeFloats := make([]float64, len(volumes))

	for i, h := range heights {
		switch t := h.(type) {
		case int:
			heightFloats[i] = float64(t)
		case uint:
			heightFloats[i] = float64(t)
		case int8:
			heightFloats[i] = float64(t)
		case uint8:
			heightFloats[i] = float64(t)
		case int16:
			heightFloats[i] = float64(t)
		case uint16:
			heightFloats[i] = float64(t)
		case int32:
			heightFloats[i] = float64(t)
		case uint32:
			heightFloats[i] = float64(t)
		case int64:
			heightFloats[i] = float64(t)
		case uint64:
			heightFloats[i] = float64(t)
		case float32:
			heightFloats[i] = float64(t)
		case float64:
			heightFloats[i] = t
		default:
			err = fmt.Errorf("The height in position %d of the heights can not be converted to a float64", i)
			return
		}
	}
	for i, h := range volumes {
		switch t := h.(type) {
		case int:
			volumeFloats[i] = float64(t)
		case uint:
			volumeFloats[i] = float64(t)
		case int8:
			volumeFloats[i] = float64(t)
		case uint8:
			volumeFloats[i] = float64(t)
		case int16:
			volumeFloats[i] = float64(t)
		case uint16:
			volumeFloats[i] = float64(t)
		case int32:
			volumeFloats[i] = float64(t)
		case uint32:
			volumeFloats[i] = float64(t)
		case int64:
			volumeFloats[i] = float64(t)
		case uint64:
			volumeFloats[i] = float64(t)
		case float32:
			volumeFloats[i] = float64(t)
		case float64:
			volumeFloats[i] = t
		default:
			err = fmt.Errorf("The volume in position %d of the volumes can not be converted to a float64", i)
			return
		}
	}

	for i := range heights {
		if i > 0 && heightFloats[i] <= heightFloats[i-1] {
			err = fmt.Errorf("The list of heights should be in increasing order, height at position %d is equal or lower than the previous one", i)
			return
		}
		if i > 0 && volumeFloats[i] <= volumeFloats[i-1] {
			err = fmt.Errorf("The list of volumes should be in increasing order, level at position %d is equal or lower than the previous one", i)
			return
		}
	}

	for i := range heights {
		if (height + sensorOffset) < heightFloats[i] {
			if i == 0 {
				break
			}
			ratioIncurrentHeight := (height + sensorOffset - heightFloats[i-1]) / (heightFloats[i] - heightFloats[i-1])
			result = ratioIncurrentHeight*(volumeFloats[i]-volumeFloats[i-1]) + volumeFloats[i-1]
			break
		}
		result = volumeFloats[i]
	}
	return
}
