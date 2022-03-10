package message

import "time"

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

func (u Update) Equals(other Update) bool {
	if u.Source != other.Source {
		return false
	}

	if len(u.Values) != len(other.Values) {
		return false
	}

	for _, v := range u.Values {
		found := false
		for _, ov := range other.Values {
			if v.Equals(ov) {
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
