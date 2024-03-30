package message

import (
	"reflect"
	"time"
)

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

func (u *Update) WithSource(s Source) *Update {
	u.Source = s
	return u
}

func (u *Update) WithTimestamp(t time.Time) *Update {
	u.Timestamp = t
	return u
}
func (u *Update) GetAndRemoveValueByPath(p string) *Value {
	for i, v := range u.Values {
		if v.Path == p {
			u.Values = append(u.Values[:i], u.Values[i+1:]...)
			return &v
		}
	}
	return nil
}
func (u *Update) AddValue(new *Value) *Update {
	// check if there is already a value with the same path in this update
	if existing := u.GetAndRemoveValueByPath(new.Path); existing != nil {
		// if the existing value and new value are a list of the same type then merge the lists
		if reflect.ValueOf(existing.Value).Kind() == reflect.Slice && reflect.ValueOf(new.Value).Kind() == reflect.Slice && reflect.TypeOf(existing.Value).Elem() == reflect.TypeOf(new.Value).Elem() {
			l1 := reflect.ValueOf(existing.Value)
			l2 := reflect.ValueOf(new.Value)
			mergedList := make([]interface{}, 0, l1.Len()+l2.Len())
			for i := 0; i < l1.Len(); i++ {
				mergedList = append(mergedList, l1.Index(i).Interface())
			}
			for i := 0; i < l2.Len(); i++ {
				mergedList = append(mergedList, l2.Index(i).Interface())
			}
			new.Value = &mergedList
		}
	}

	u.Values = append(u.Values, *new)
	return u
}

func (u Update) Equals(other Update) bool {
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
