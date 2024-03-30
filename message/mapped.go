package message

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

type Mapped struct {
	Context string   `json:"context"` // indicates what the data is about
	Origin  string   `json:"origin"`  // indicates the creator of the data
	Updates []Update `json:"updates"`
}

func NewMapped() *Mapped {
	return &Mapped{
		Updates: make([]Update, 0),
	}
}

func (m *Mapped) WithContext(c string) *Mapped {
	m.Context = c
	return m
}

func (m *Mapped) WithOrigin(o string) *Mapped {
	m.Origin = o
	return m
}

func (m *Mapped) AddUpdate(u *Update) *Mapped {
	m.Updates = append(m.Updates, *u)
	return m
}

func (m Mapped) Equals(other Mapped) bool {
	if m.Context != other.Context {
		return false
	}

	if m.Origin != other.Origin {
		return false
	}

	if len(m.Updates) != len(other.Updates) {
		return false
	}

	for _, u := range m.Updates {
		found := false
		for _, ou := range other.Updates {
			if u.Equals(ou) {
				found = true
				continue
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (m Mapped) ToSingleValueMapped() []SingleValueMapped {
	result := make([]SingleValueMapped, 0)

	for _, u := range m.Updates {
		for _, v := range u.Values {
			s := SingleValueMapped{
				Context:   m.Context,
				Origin:    m.Origin,
				Source:    u.Source,
				Timestamp: u.Timestamp,
				Path:      v.Path,
				Value:     v.Value,
			}

			result = append(result, s)
		}
	}
	return result
}

type SingleValueMapped struct {
	Context   string      `json:"context"`
	Origin    string      `json:"origin"`
	Source    Source      `json:"source"`
	Timestamp time.Time   `json:"timestamp"`
	Path      string      `json:"path"`
	Value     interface{} `json:"value"`
}

func NewSingleValueMapped() *SingleValueMapped {
	return &SingleValueMapped{}
}

func (s SingleValueMapped) ToMapped() *Mapped {
	v := NewValue().WithPath(s.Path).WithValue(s.Value)
	u := NewUpdate().WithSource(s.Source).WithTimestamp(s.Timestamp).AddValue(v)
	m := NewMapped().WithContext(s.Context).WithOrigin(s.Origin).AddUpdate(u)
	return m
}

func (s SingleValueMapped) Equals(other SingleValueMapped) bool {
	return s.Context == other.Context && s.Path == other.Path && reflect.DeepEqual(s.Value, other.Value)
}

// Merges left with right, if both left and right have the same property the value of the right property will be returned
func (left SingleValueMapped) Merge(right SingleValueMapped) SingleValueMapped {
	leftMerger, ok := left.Value.(Merger)
	if !ok {
		return right
	}
	rightMerger, ok := right.Value.(Merger)
	if !ok {
		return right
	}

	result, err := leftMerger.Merge(rightMerger)
	if err != nil {
		return right
	}

	return SingleValueMapped{
		Context:   right.Context,
		Origin:    right.Origin,
		Source:    right.Source,
		Timestamp: right.Timestamp,
		Path:      right.Path,
		Value:     result,
	}
}

func (s *SingleValueMapped) UnmarshalJSON(data []byte) error {
	var err error
	var j map[string]interface{}
	if err = json.Unmarshal(data, &j); err != nil {
		return err
	}
	for _, key := range []string{"context", "origin", "source", "timestamp", "path", "value"} {
		if _, ok := j[key]; !ok {
			return fmt.Errorf("the key '%v' is missing in the json message %+v", key, j)
		}
	}

	str, ok := j["context"].(string)
	if !ok {
		return fmt.Errorf("can't convert %v to a string", j["context"])
	}
	s.Context = str

	str, ok = j["origin"].(string)
	if !ok {
		return fmt.Errorf("can't convert %v to a string", j["origin"])
	}
	s.Origin = str

	var bytes []byte
	if bytes, err = json.Marshal(j["source"]); err != nil {
		return fmt.Errorf("can't convert %v to a message.Source", j["source"])
	}
	if err = json.Unmarshal(bytes, &s.Source); err != nil {
		return fmt.Errorf("can't convert %v to a message.Source", j["source"])
	}

	str, ok = j["timestamp"].(string)
	if !ok {
		return fmt.Errorf("can't convert %v to a time.Time", j["timestamp"])
	}
	t, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		return fmt.Errorf("can't convert %v to a time.Time", j["timestamp"])
	}
	s.Timestamp = t

	str, ok = j["path"].(string)
	if !ok {
		return fmt.Errorf("can't convert %v to a string", j["path"])
	}
	s.Path = str

	if decoded, err := Decode(j["value"]); err == nil {
		s.Value = decoded
		return nil
	}

	return fmt.Errorf("don't know how to unmarshal %v", string(data))
}
