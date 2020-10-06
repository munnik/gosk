package signalk

// Position2D is used for position values without altitude
type Position2D struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

// Position3D is used for position values with altitude
type Position3D struct {
	Position2D
	Altitude float64 `json:"altitude"`
}

// Value is part of an Update
type Value struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// Update is part of a Delta
type Update struct {
	Source    string  `json:"source"`
	Timestamp string  `json:"timestamp"`
	Values    []Value `json:"values"`
}

// Delta as specified in https://signalk.org/specification/1.4.0/doc/data_model.html
type Delta struct {
	Context string   `json:"context"`
	Updates []Update `json:"updates"`
}

// AppendValue adds a value to an update
func (u *Update) AppendValue(v Value) {
	u.Values = append(u.Values, v)
}
