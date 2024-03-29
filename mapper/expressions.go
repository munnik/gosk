package mapper

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

type ExpressionEnvironment map[string]any

var virtualMachine = vm.VM{}

func NewExpressionEnvironment() ExpressionEnvironment {
	return ExpressionEnvironment{
		"currentToRatio":   CurrentToRatio,
		"pressureToHeight": PressureToHeight,
		"heightToVolume":   HeightToVolume,
		"movingAverage":    MovingAverage,
		"copySign":         CopySign,
		"powerW":           PowerW,
		"toFloat":          ToFloat,
		"bitwiseAnd":       BitwiseAnd,
		"bitwiseOr":        BitwiseOr,
		"bitwiseXor":       BitwiseXor,
		"bitwiseNot":       BitwiseNot,
		"bitwiseContains":  BitwiseContains,
		"isBitSet":         IsBitSet,
		"notify":           Notify,
	}
}

// Returns the 4-20mA input signal to a ratio, 4000uA => 0.0, 8000uA => 0.25, 12000uA => 0.5, 16000uA => 0.75, 20000uA => 1.0
// current is in uA (1000000uA is 1A)
// return value is a ratio (0.0 .. 1.0)
func CurrentToRatio(current float64) float64 {
	return (current - 4000) / 16000
}

// Converts a pressure and density to a height
// pressure is in Pa (1 Bar is 100000 Pascal)
// density is in kg/m3 (typical value for diesel is 840)
// return value is in m
func PressureToHeight(pressure float64, density float64) float64 {
	G := 9.8 // acceleration due to gravity
	return pressure / (density * G)
}

// Returns the HeightToVolume corresponding to the measured height. This function is used when a pressure sensor is used in a tank.
// height is in m
// sensorOffset is in m (positive means that the sensor is placed above the bottom of the tank, negative value means that the sensor is placed below the tank)
// heights is in m, list of heights with corresponding volumes
// volumes is in m3, list of volumes with corresponding heights
// return value is in m3
func HeightToVolume(height float64, sensorOffset float64, heights []interface{}, volumes []interface{}) (result float64, err error) {
	if len(heights) != len(volumes) {
		err = fmt.Errorf("the list of heights should have the same length as the list of volumes, the lengths are %d and %d", len(heights), len(volumes))
		return
	}

	heightFloats, err := ListToFloats(heights)
	if err != nil {
		return 0, err
	}
	volumeFloats, err := ListToFloats(volumes)
	if err != nil {
		return 0, err
	}

	for i := range heights {
		if i > 0 && heightFloats[i] <= heightFloats[i-1] {
			err = fmt.Errorf("the list of heights should be in increasing order, height at position %d is equal or lower than the previous one", i)
			return
		}
		if i > 0 && volumeFloats[i] <= volumeFloats[i-1] {
			err = fmt.Errorf("the list of volumes should be in increasing order, level at position %d is equal or lower than the previous one", i)
			return
		}
	}

	for i := range heights {
		if (height + sensorOffset) < heightFloats[i] {
			if i == 0 {
				return
			}
			ratioIncurrentHeight := (height + sensorOffset - heightFloats[i-1]) / (heightFloats[i] - heightFloats[i-1])
			result = ratioIncurrentHeight*(volumeFloats[i]-volumeFloats[i-1]) + volumeFloats[i-1]
			return
		}
		result = volumeFloats[i]
	}
	return
}

func ListToFloats(input []interface{}) ([]float64, error) {
	result := make([]float64, len(input))

	for i, h := range input {
		switch t := h.(type) {
		case int:
			result[i] = float64(t)
		case uint:
			result[i] = float64(t)
		case int8:
			result[i] = float64(t)
		case uint8:
			result[i] = float64(t)
		case int16:
			result[i] = float64(t)
		case uint16:
			result[i] = float64(t)
		case int32:
			result[i] = float64(t)
		case uint32:
			result[i] = float64(t)
		case int64:
			result[i] = float64(t)
		case uint64:
			result[i] = float64(t)
		case float32:
			result[i] = float64(t)
		case float64:
			result[i] = t
		default:
			return []float64{}, fmt.Errorf("the value in position %d of the input can not be converted to a float64", i)
		}
	}
	return result, nil
}

// calculate the average of all historical values stored for this path
func MovingAverage(values []message.SingleValueMapped) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v.Value.(float64)
	}
	return sum / float64(len(values))
}

// calculate the power based on rotations and torque
// is always a positive value
func PowerW(rotations float64, torque float64) float64 {
	return math.Abs(2 * math.Pi * rotations * torque)
}

func CopySign(f float64, sign float64) float64 {
	return math.Copysign(f, sign)

}

func ToFloat(mostSignificant, leastSignificant uint16) float32 {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], mostSignificant)
	binary.BigEndian.PutUint16(data[2:4], leastSignificant)

	bits := binary.BigEndian.Uint32(data)
	return math.Float32frombits(bits)
}

func BitwiseAnd(left, right uint16) uint16 {
	return left & right
}

func BitwiseOr(left, right uint16) uint16 {
	return left | right
}

func BitwiseXor(left, right uint16) uint16 {
	return left ^ right
}

func BitwiseNot(left uint16) uint16 {
	return ^left
}

func BitwiseContains(input, pattern uint16) bool {
	return (input & pattern) == pattern
}

func IsBitSet(input uint16, position int) bool {
	return BitwiseContains(input, 1<<position)
}

func Notify(s bool, m string) message.Notification {
	return message.Notification{
		State:   &s,
		Message: &m,
	}
}

func runExpr(env ExpressionEnvironment, mappingConfig *config.MappingConfig) (interface{}, error) {
	for key, value := range mappingConfig.ExpressionEnvironment {
		env[key] = value
	}

	if mappingConfig.CompiledExpression == nil {
		// TODO: each iteration the CompiledExpression is nil
		var err error
		if mappingConfig.CompiledExpression, err = expr.Compile(mappingConfig.Expression); err != nil {
			logger.GetLogger().Warn(
				"Could not compile the mapping expression",
				zap.String("Expression", mappingConfig.Expression),
				zap.String("Error", err.Error()),
			)
			return nil, err
		}
	}
	// the compiled program exists, let's run it
	output, err := virtualMachine.Run(mappingConfig.CompiledExpression, env)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not run the mapping expression",
			zap.String("Expression", mappingConfig.Expression),
			zap.String("Environment", fmt.Sprintf("%+v", env)),
			zap.String("Error", err.Error()),
		)
		return nil, err
	}

	// the value is a map so we could try to decode it
	if m, ok := output.(map[string]interface{}); ok {
		if decoded, err := message.Decode(m); err == nil {
			output = decoded
		}
	}

	return output, nil
}

func swapPointAndComma(input string) string {
	result := []rune(input)

	for i := range result {
		if result[i] == '.' {
			result[i] = ','
		} else if result[i] == ',' {
			result[i] = '.'
		}
	}
	return string(result)
}
