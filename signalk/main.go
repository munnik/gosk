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

// OverallLength is used for the overall length of a vessel
type OverallLength struct {
	Overall float64 `json:"overall"`
}

// HullLength is used for the hull length of a vessel
type HullLength struct {
	Hull float64 `json:"hull"`
}

// WaterlineLength is used for the water length of a vessel
type WaterlineLength struct {
	Waterline float64 `json:"waterline"`
}

// OverallAndHullLength is used for the combination of these lengths
type OverallAndHullLength struct {
	OverallLength
	HullLength
}

// OverallAndWaterlineLength is used for the combination of these lengths
type OverallAndWaterlineLength struct {
	OverallLength
	WaterlineLength
}

// HullAndWaterlineLength is used for the combination of these lengths
type HullAndWaterlineLength struct {
	HullLength
	WaterlineLength
}

// Length is used for the combination of all lengths
type Length struct {
	OverallLength
	HullLength
	WaterlineLength
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

// DeltaWithoutContext as specified in https://signalk.org/specification/1.4.0/doc/data_model.html
type DeltaWithoutContext struct {
	Updates []Update `json:"updates"`
}

// DeltaWithContext as specified in https://signalk.org/specification/1.4.0/doc/data_model.html
type DeltaWithContext struct {
	Context string   `json:"context"`
	Updates []Update `json:"updates"`
}

// Delta as specified in https://signalk.org/specification/1.4.0/doc/data_model.html
type Delta interface {
	AppendValue(v Value)
}

// AppendValue adds a value to a delta
func (d DeltaWithoutContext) AppendValue(v Value) {
	d.Updates[0].Values = append(d.Updates[0].Values, v)
}

// AppendValue adds a value to a delta
func (d DeltaWithContext) AppendValue(v Value) {
	d.Updates[0].Values = append(d.Updates[0].Values, v)
}
