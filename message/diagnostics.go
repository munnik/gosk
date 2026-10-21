package message

import (
	"fmt"
	"math"
	"reflect"
)

// Diagnostics describe the reading a value was mapped from, rather than the
// quantity it measures: the raw loop current behind a tank level, whether
// the input module still sees a sensor, why a reply was rejected. They
// travel with the value they belong to - written, rate limited, transferred
// and stored as one sample - instead of as separate paths that only line
// up with it by timestamp.
//
// The field is a flat object of finite numbers, booleans and strings (see
// NormalizeDiagnostic). It is gosk's own and has no counterpart in a
// Signal K delta, which is why the Signal K writers leave it out; Signal K's
// "meta" is something else again - static units, zones and display names
// per path.
//
// A value may carry diagnostics and no value at all: that is how a mapper
// says it received a reading and rejected it (see DiagnosticRejected),
// where it used to say nothing.

// DiagnosticRejected is the diagnostic a mapper sets, on a value with no
// value, when it received a reading and rejected it as a suspected device
// fault. It holds the reason, e.g. "all zeros".
const DiagnosticRejected = "rejected"

// NormalizeDiagnostic returns v as one of the types a diagnostic may hold -
// float64, bool or string - or an error. Every integer type becomes a
// float64, the way JSON and Signal K represent any number, so a diagnostic
// reads the same after a round trip through JSON, msgpack or the database
// as it did when its expression produced it. A NaN or an infinity is
// rejected: JSON cannot represent either, and the database writer would
// otherwise drop the whole row the diagnostic rides on.
func NormalizeDiagnostic(v any) (any, error) {
	switch val := v.(type) {
	case bool, string:
		return val, nil
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil, fmt.Errorf("%v is not a finite number", val)
		}
		return val, nil
	case float32:
		return NormalizeDiagnostic(float64(val))
	case int:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	case nil:
		return nil, fmt.Errorf("a diagnostic cannot be nil")
	default:
		return nil, fmt.Errorf("a diagnostic must be a number, a bool or a string, not %T", v)
	}
}

// normalizeDiagnostics applies NormalizeDiagnostic to every entry of d that
// arrived from outside - a JSON message, a database row - dropping the ones
// that do not normalise, and returns nil for an empty result. It never
// fails: like Value.UnmarshalJSON, it keeps what it can rather than reject a
// message for a field this version cannot interpret.
func normalizeDiagnostics(d map[string]any) map[string]any {
	if len(d) == 0 {
		return nil
	}
	result := make(map[string]any, len(d))
	for k, v := range d {
		if n, err := NormalizeDiagnostic(v); err == nil {
			result[k] = n
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// diagnosticsEqual treats a nil and an empty set of diagnostics as the same:
// neither is written to any wire format.
func diagnosticsEqual(a, b map[string]any) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
