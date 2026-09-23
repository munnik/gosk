package mapper

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

type ExpressionEnvironment map[string]any

// runVM runs a compiled expression program in a fresh vm.VM. A vm.VM isn't
// safe to reuse across concurrent calls - it holds a mutable execution
// stack (see its Stack/Scopes/scopePool fields) that Run mutates in place
// - so this must never become a single shared instance again: gosk's
// mapper stages call runExpr (and therefore this) from multiple goroutines
// at once wherever a mapper is sharded for concurrency (see shard.go), and
// a shared VM there previously produced silently wrong computed values
// under concurrent load rather than a crash, since two goroutines'
// interleaved pushes/pops onto the same stack still produce *a* value, just
// not the right one. A fresh vm.VM{} is a cheap zero-value struct - its
// slices grow lazily as the program executes - so there's no meaningful
// cost to not reusing one.
func runVM(program *vm.Program, env any) (any, error) {
	return (&vm.VM{}).Run(program, env)
}

func NewExpressionEnvironment() ExpressionEnvironment {
	return ExpressionEnvironment{
		"currentToRatio":     CurrentToRatio,
		"pressureToHeight":   PressureToHeight,
		"heightToVolume":     HeightToVolume,
		"movingAverage":      MovingAverage,
		"copySign":           CopySign,
		"powerW":             PowerW,
		"toFloat":            ToFloat,
		"toUInt":             ToUInt16,
		"toInt":              ToInt16,
		"toUInt32":           ToUInt32,
		"toInt32":            ToInt32,
		"bitwiseAnd":         BitwiseAnd,
		"bitwiseOr":          BitwiseOr,
		"bitwiseXor":         BitwiseXor,
		"bitwiseNot":         BitwiseNot,
		"bitwiseContains":    BitwiseContains,
		"isBitSet":           IsBitSet,
		"between":            Between,
		"random":             Random,
		"float64ToRegisters": Float64ToRegisters,
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

// Returns true if value is between min and max, both bounds are inclusive.
// value, min and max can be any numeric type, they do not need to be float64.
func Between(value, min, max any) (bool, error) {
	floats, err := ListToFloats([]any{value, min, max})
	if err != nil {
		return false, err
	}
	return floats[0] >= floats[1] && floats[0] <= floats[2], nil
}

// Returns a pseudo random number in the half open interval [0.0, 1.0), so
// 0.0 is a possible result but 1.0 is not. Takes no arguments.
//
// It reads the global source of math/rand/v2, which is safe to use from
// several goroutines at once - mappers call expressions concurrently
// wherever they are sharded, see the comment on runVM. The numbers are not
// suitable for anything that needs to be unguessable.
func Random() float64 {
	return rand.Float64()
}

func ToFloat(mostSignificant, leastSignificant uint16) float32 {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], mostSignificant)
	binary.BigEndian.PutUint16(data[2:4], leastSignificant)

	bits := binary.BigEndian.Uint32(data)
	return math.Float32frombits(bits)
}

func ToUInt16(mostSignificant, leastSignificant uint8) uint16 {
	data := make([]byte, 2)
	data[0] = mostSignificant
	data[1] = leastSignificant
	return binary.BigEndian.Uint16(data)
}

func ToInt16(unsigned uint16) int16 {
	return int16(unsigned)
}

func ToUInt32(mostSignificant, leastSignificant uint16) uint32 {
	return uint32(mostSignificant)*65536 + uint32(leastSignificant)
}

func ToInt32(mostSignificant, leastSignificant uint16) int32 {
	return int32(uint32(mostSignificant)*65536 + uint32(leastSignificant))
}
func Float64ToRegisters(value float64) []interface{} {
	res := make([]interface{}, 4)
	n := math.Float64bits(value)
	res[0] = int(uint16(n >> 48))
	res[1] = int(uint16(n >> 32))
	res[2] = int(uint16(n >> 16))
	res[3] = int(uint16(n))
	return res
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

func runExpr(env ExpressionEnvironment, mappingConfig *config.MappingConfig) (interface{}, error) {
	for key, value := range mappingConfig.ExpressionEnvironment {
		env[key] = value
	}

	if mappingConfig.CompiledExpression == nil {

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
	output, err := runVM(mappingConfig.CompiledExpression, env)
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

	// A NaN or Inf result (e.g. a division by a zero delta on a mapper's
	// first-ever evaluation, before it has a previous sample to diff
	// against) can never be JSON-marshalled - encoding/json rejects both.
	// Left as a normal value, that doesn't just fail to report this one
	// path: nanomsg.Publisher.Send marshals the whole message.Mapped at
	// once, so one bad value silently drops every other value alongside
	// it too, and - since a Publisher's sdnotify.Ready only fires after a
	// successful send - a mapper whose first output happens to include
	// one can never start at all under Type=notify, no matter how long
	// it's given. Reject it here instead, the same way a failed compile
	// or run is already rejected, so the caller skips just this one path
	// and keeps whatever else it computed.
	if f, ok := output.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
		err := fmt.Errorf("expression result is %v, not a finite number", f)
		logger.GetLogger().Warn(
			"Discarding a non-finite mapping expression result",
			zap.String("Expression", mappingConfig.Expression),
			zap.String("Environment", fmt.Sprintf("%+v", env)),
			zap.String("Error", err.Error()),
		)
		return nil, err
	}

	return output, nil
}

func runTimestampExpr(env ExpressionEnvironment, mappingConfig *config.MappingConfig) (interface{}, error) {
	for key, value := range mappingConfig.ExpressionEnvironment {
		env[key] = value
	}

	if mappingConfig.CompiledTimestampExpression == nil {

		var err error
		if mappingConfig.CompiledTimestampExpression, err = expr.Compile(mappingConfig.TimestampExpression); err != nil {
			logger.GetLogger().Warn(
				"Could not compile the timestamp expression",
				zap.String("Expression", mappingConfig.TimestampExpression),
				zap.String("Error", err.Error()),
			)
			return nil, err
		}
	}
	// the compiled program exists, let's run it
	output, err := runVM(mappingConfig.CompiledTimestampExpression, env)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not run the timestamp expression",
			zap.String("Expression", mappingConfig.TimestampExpression),
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
