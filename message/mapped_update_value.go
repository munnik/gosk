package message

import (
	"encoding/json"
	"fmt"
)

type Value struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
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

	if decoded, err := Decode(j["value"]); err == nil {
		v.Value = decoded
		return nil
	}

	return fmt.Errorf("don't know how to unmarshal %v", string(data))
}

func (v Value) Equals(other Value) bool {
	return v.Path == other.Path && v.Value == other.Value
}
