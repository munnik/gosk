package message

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

type Value struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
	// Diagnostics describe the reading Value was mapped from - see
	// diagnostics.go. Nil when there are none, which is never written.
	Diagnostics map[string]any `json:"diagnostics,omitempty"`
}

func NewValue() *Value {
	return &Value{}
}

func (v *Value) WithPath(p string) *Value {
	v.Path = p
	return v
}

func (v *Value) WithValue(val interface{}) *Value {
	v.Value = val
	return v
}

func (v *Value) WithDiagnostics(d map[string]any) *Value {
	v.Diagnostics = d
	return v
}

func (v *Value) UnmarshalJSON(data []byte) error {
	var err error
	var j map[string]interface{}
	if err = json.Unmarshal(data, &j); err != nil {
		return err
	}
	for _, key := range []string{"path", "value"} {
		if _, ok := j[key]; !ok {
			return fmt.Errorf("the key '%v' is missing in the json message %+v", key, j)
		}
	}

	s, ok := j["path"].(string)
	if !ok {
		return fmt.Errorf("can't convert %v to a string", j["path"])
	}
	v.Path = s

	// A value whose shape this version does not recognise is kept as it
	// arrived rather than rejected. Rejecting it fails the whole
	// message, and a message is never alone: reader/mqtt.go unmarshals a
	// batch of hundreds of them in one call, so one unknown value used to
	// discard every other message travelling with it - engine data, AIS
	// targets, positions, all of it.
	//
	// That is not hypothetical. Adding the Current type (see
	// environment.current in mapper/meteohydro.go) to vessels whose cloud
	// side was still a version older than that type did exactly this,
	// silently, to three vessels at once. A gosk that does not know a
	// type can still carry it, store it and pass it on - it just cannot
	// interpret it - and that is far better than dropping the batch.
	decoded, err := Decode(j["value"])
	if err != nil {
		logger.GetLogger().Warn(
			"Keeping a value this version does not know how to decode",
			zap.String("Path", v.Path),
			zap.String("Error", err.Error()),
		)
	}
	v.Value = decoded

	// Optional, and only ever kept, never a reason to fail: a message
	// from a gosk that predates diagnostics has none, and one this
	// version cannot read is dropped for the same reason an unknown
	// value is kept above - the batch it travels in matters more.
	if raw, ok := j["diagnostics"]; ok && raw != nil {
		if d, ok := raw.(map[string]interface{}); ok {
			v.Diagnostics = normalizeDiagnostics(d)
		} else {
			logger.GetLogger().Warn(
				"Ignoring diagnostics that are not a JSON object",
				zap.String("Path", v.Path),
				zap.Any("Diagnostics", raw),
			)
		}
	}

	return nil
}

func (v Value) Equals(other Value) bool {
	return v.Path == other.Path && reflect.DeepEqual(v.Value, other.Value) && diagnosticsEqual(v.Diagnostics, other.Diagnostics)
}
