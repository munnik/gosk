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
func heightToVolume(height float64, sensorOffset float64, heights []float64, volumes []float64) (result float64, err error) {
	if len(heights) != len(volumes) {
		err = fmt.Errorf("The list of heights should have the same length as the list of volumes, the lengths are %d and %d", len(heights), len(volumes))
		return
	}

	for i := range heights {
		if i > 0 && heights[i] <= heights[i-1] {
			err = fmt.Errorf("The list of heights should be in increasing order, height at position %d is equal or lower than the previous one", i)
			return
		}
		if i > 0 && volumes[i] <= volumes[i-1] {
			err = fmt.Errorf("The list of volumes should be in increasing order, level at position %d is equal or lower than the previous one", i)
			return
		}
	}

	for i := range heights {
		if (height + sensorOffset) < heights[i] {
			if i == 0 {
				break
			}
			ratioIncurrentHeight := (height + sensorOffset - heights[i-1]) / (heights[i] - heights[i-1])
			result = ratioIncurrentHeight*(volumes[i]-volumes[i-1]) + volumes[i-1]
			break
		}
		result = volumes[i]
	}
	return
}
