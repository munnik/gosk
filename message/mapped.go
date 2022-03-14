package message

import (
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
			result = append(result, SingleValueMapped{
				Context:   m.Context,
				Origin:    m.Origin,
				Source:    u.Source,
				Timestamp: u.Timestamp,
				Path:      v.Path,
				Value:     v.Value,
			})
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

func (s SingleValueMapped) ToMapped() Mapped {
	v := NewValue().WithPath(s.Path).WithValue(s.Value)
	u := NewUpdate().WithSource(s.Source).WithTimestamp(s.Timestamp).AddValue(v)
	m := NewMapped().WithContext(s.Context).WithOrigin(s.Origin).AddUpdate(u)
	return *m
}

func (s SingleValueMapped) Equals(other SingleValueMapped) bool {
	return s.Context == other.Context && s.Path == other.Path && s.Value == other.Value
}
