package message

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
)

type RawExtraInfo interface{}

type Raw struct {
	Collector string
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Uuid      uuid.UUID `json:"uuid"`
	Value     []byte    `json:"value"`
}

func NewRaw() *Raw {
	return &Raw{
		Uuid:      uuid.New(),
		Timestamp: time.Now(),
	}
}

func (r *Raw) WithCollector(c string) *Raw {
	r.Collector = c
	return r
}

func (r *Raw) WithType(t string) *Raw {
	r.Type = t
	return r
}

func (r *Raw) WithValue(v []byte) *Raw {
	r.Value = v
	return r
}

func (r Raw) MarshalJSON() ([]byte, error) {
	var result map[string]string = make(map[string]string)
	result["collector"] = r.Collector
	result["timestamp"] = r.Timestamp.UTC().Format(time.RFC3339Nano)
	result["type"] = r.Type
	result["uuid"] = r.Uuid.String()
	result["value"] = base64.StdEncoding.EncodeToString(r.Value)
	return json.Marshal(&result)
}

func (r *Raw) UnmarshalJSON(data []byte) error {
	var err error
	var j map[string]string
	if err = json.Unmarshal(data, &j); err != nil {
		return err
	}
	for _, key := range []string{"collector", "timestamp", "type", "uuid", "value"} {
		if _, ok := j[key]; !ok {
			return fmt.Errorf("the key '%v' is missing in the json message %+v", key, j)
		}
	}
	r.Collector = j["collector"]
	if r.Timestamp, err = time.Parse(time.RFC3339Nano, j["timestamp"]); err != nil {
		return err
	}
	r.Type = j["type"]
	if r.Uuid, err = uuid.Parse(j["uuid"]); err != nil {
		return err
	}
	if r.Value, err = base64.StdEncoding.DecodeString(j["value"]); err != nil {
		return err
	}

	return nil
}

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

type Update struct {
	Source    Source    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	Values    []Value   `json:"values"`
}

func NewUpdate() *Update {
	return &Update{
		Timestamp: time.Now(),
		Values:    make([]Value, 0),
	}
}

func (u *Update) WithSource(s *Source) *Update {
	u.Source = *s
	return u
}

func (u *Update) WithTimestamp(t time.Time) *Update {
	u.Timestamp = t
	return u
}

func (u *Update) AddValue(v *Value) *Update {
	u.Values = append(u.Values, *v)
	return u
}

type Source struct {
	Label string `json:"label"`
	Type  string `json:"type"`
}

func NewSource() *Source {
	return &Source{}
}

func (s *Source) WithLabel(l string) *Source {
	s.Label = l
	return s
}

func (s *Source) WithType(t string) *Source {
	s.Type = t
	return s
}

type Value struct {
	Path  string      `json:"path"`
	Uuid  uuid.UUID   `json:"uuid"`
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

func (v *Value) WithUuid(u uuid.UUID) *Value {
	v.Uuid = u
	return v
}

func (v Value) MarshalJSON() ([]byte, error) {
	var result map[string]interface{} = make(map[string]interface{})
	result["path"] = v.Path
	result["uuid"] = v.Uuid.String()
	result["value"] = v.Value
	return json.Marshal(&result)
}

func (v *Value) UnmarshalJSON(data []byte) error {
	var err error
	var j map[string]interface{}
	if err = json.Unmarshal(data, &j); err != nil {
		return err
	}
	for _, key := range []string{"path", "uuid", "value"} {
		if _, ok := j[key]; !ok {
			return fmt.Errorf("the key '%v' is missing in the json message %+v", key, j)
		}
	}

	s, ok := j["path"].(string)
	if !ok {
		return fmt.Errorf("can't convert %v to a string", j["path"])
	}
	v.Path = s

	s, ok = j["uuid"].(string)
	if !ok {
		return fmt.Errorf("can't convert %v to a string", j["path"])
	}
	if v.Uuid, err = uuid.Parse(s); err != nil {
		return err
	}

	if f, ok := j["value"].(int64); ok {
		v.Value = f
		return nil
	}
	if f, ok := j["value"].(float64); ok {
		v.Value = f
		return nil
	}
	if s, ok = j["value"].(string); ok {
		v.Value = s
		return nil
	}

	if decoded, err := Decode(j["value"]); err == nil {
		v.Value = decoded
		return nil
	}

	return fmt.Errorf("don't know how to unmarshal %v", string(data))
}

type Position struct {
	Altitude  *float64 `json:"altitude,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

type Length struct {
	Overall   *float64 `json:"overall,omitempty"`
	Hull      *float64 `json:"hull,omitempty"`
	Waterline *float64 `json:"waterline,omitempty"`
}

type Alarm struct {
	State   bool   `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

type Buffer struct {
	Deltas []Mapped `json:"deltas"`
	lock   *sync.RWMutex
}

func NewBuffer() *Buffer {
	return &Buffer{
		Deltas: make([]Mapped, 0),
		lock:   &sync.RWMutex{},
	}
}

func (b *Buffer) Lock() *Buffer {
	b.lock.Lock()
	return b
}

func (b *Buffer) Unlock() *Buffer {
	b.lock.Unlock()
	return b
}

func (b *Buffer) Empty() *Buffer {
	b.Deltas = make([]Mapped, 0)
	return b
}

func (b *Buffer) Append(m Mapped) *Buffer {
	b.Deltas = append(b.Deltas, m)
	return b
}

func (b Buffer) MarshalJSON() ([]byte, error) {
	var result map[string]interface{} = make(map[string]interface{})
	result["deltas"] = b.Deltas
	return json.Marshal(result)
}

func Decode(input interface{}) (interface{}, error) {
	var metadata mapstructure.Metadata
	p := Position{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &p, &metadata); err == nil && len(metadata.Unused) == 0 {
		return p, nil
	}

	l := Length{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &l, &metadata); err == nil && len(metadata.Unused) == 0 {
		return l, nil
	}

	a := Alarm{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &a, &metadata); err == nil && len(metadata.Unused) == 0 {
		return a, nil
	}

	return nil, fmt.Errorf("don't know how to decode %v", input)
}
