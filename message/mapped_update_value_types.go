package message

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
)

type Merger interface {
	// Merges left with right, if both left and right have the same property the value of the right property will be returned
	Merge(right Merger) (Merger, error)
}

// msgp codegen for every concrete type Value.Value can hold, used by
// value_msgp.go's tagged encoding to skip Decode()'s trial-and-error on
// gosk's internal nanomsg transport: since MarshalMsg picks the tag from
// an actual Go type switch (not a guess), the matching UnmarshalMsg
// branch decodes straight into the right type, no probing needed. Adding
// a new type to Decode()'s candidate list must also add it to
// value_msgp.go's marshalValueMsg/unmarshalValueMsg switches, or it
// silently falls back to the (still-correct, just slower) JSON+Decode()
// path there.
//
//go:generate msgp -tests=false -o=mapped_update_value_types_msgp_gen.go
type VesselType struct {
	Id          *int    `json:"id,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (left VesselType) Merge(right Merger) (Merger, error) {
	var err error
	if right, ok := right.(VesselType); !ok {
		err = fmt.Errorf("right has type %T but should be type %T", right, left)
	} else {
		if right.Id != nil {
			left.Id = right.Id
		}
		if right.Description != nil {
			left.Description = right.Description
		}
	}
	return left, err
}

type VesselInfo struct {
	MMSI *string `json:"mmsi,omitempty"`
	Name *string `json:"name,omitempty"`
}

func (left VesselInfo) Merge(right Merger) (Merger, error) {
	var err error
	if right, ok := right.(VesselInfo); !ok {
		err = fmt.Errorf("right has type %T but should be type %T", right, left)
	} else {
		if right.MMSI != nil {
			left.MMSI = right.MMSI
		}
		if right.Name != nil {
			left.Name = right.Name
		}
	}
	return left, err
}

type Position struct {
	Altitude  *float64 `json:"altitude,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

func (left Position) Merge(right Merger) (Merger, error) {
	var err error
	if right, ok := right.(Position); !ok {
		err = fmt.Errorf("right has type %T but should be type %T", right, left)
	} else {
		if right.Altitude != nil {
			left.Altitude = right.Altitude
		}
		if right.Latitude != nil {
			left.Latitude = right.Latitude
		}
		if right.Longitude != nil {
			left.Longitude = right.Longitude
		}
	}
	return left, err
}

type Length struct {
	Overall   *float64 `json:"overall,omitempty"`
	Hull      *float64 `json:"hull,omitempty"`
	Waterline *float64 `json:"waterline,omitempty"`
}

func (left Length) Merge(right Merger) (Merger, error) {
	var err error
	if right, ok := right.(Length); !ok {
		err = fmt.Errorf("right has type %T but should be type %T", right, left)
	} else {
		if right.Overall != nil {
			left.Overall = right.Overall
		}
		if right.Hull != nil {
			left.Hull = right.Hull
		}
		if right.Waterline != nil {
			left.Waterline = right.Waterline
		}
	}
	return left, err
}

type Notification struct {
	State   *string  `json:"state,omitempty"`
	Method  []string `json:"method,omitempty"`
	Message *string  `json:"message,omitempty"`
}

func (left Notification) Merge(right Merger) (Merger, error) {
	var err error
	if right, ok := right.(Notification); !ok {
		err = fmt.Errorf("right has type %T but should be type %T", right, left)
	} else {
		if right.State != nil {
			left.State = right.State
		}
		if len(right.Method) != 0 {
			left.Method = right.Method
		}
		if right.Message != nil {
			left.Message = right.Message
		}
	}
	return left, err
}

type Draft struct {
	Current          *float64  `json:"current,omitempty"`
	CurrentPort      []float64 `json:"currentPort,omitempty"`
	CurrentStarboard []float64 `json:"currentStarboard,omitempty"`
}

func (left Draft) Merge(right Merger) (Merger, error) {
	var err error
	if right, ok := right.(Draft); !ok {
		err = fmt.Errorf("right has type %T but should be type %T", right, left)
	} else {
		if right.Current != nil {
			left.Current = right.Current
		}
		if len(right.CurrentPort) != 0 {
			left.CurrentPort = right.CurrentPort
		}
		if len(right.CurrentStarboard) != 0 {
			left.CurrentStarboard = right.CurrentStarboard
		}
	}
	return left, err
}

// Current is the set and drift of the current affecting the vessel, see
// environment.current in the Signal K specification. SetTrue and
// SetMagnetic are in rad, Drift in m/s.
type Current struct {
	Drift       *float64 `json:"drift,omitempty"`
	SetTrue     *float64 `json:"setTrue,omitempty"`
	SetMagnetic *float64 `json:"setMagnetic,omitempty"`
}

func (left Current) Merge(right Merger) (Merger, error) {
	var err error
	if right, ok := right.(Current); !ok {
		err = fmt.Errorf("right has type %T but should be type %T", right, left)
	} else {
		if right.Drift != nil {
			left.Drift = right.Drift
		}
		if right.SetTrue != nil {
			left.SetTrue = right.SetTrue
		}
		if right.SetMagnetic != nil {
			left.SetMagnetic = right.SetMagnetic
		}
	}
	return left, err
}

type Coefficient struct {
	Magnitude float64 `json:"magnitude"`
	Phase     float64 `json:"phase"`
}
type Spectrum struct {
	NumberOfSamples   int           `json:"numberOfSamples"`
	FrequencyStepSize float64       `json:"frequencyStepSize"`
	Duration          float64       `json:"duration"`
	Coefficients      []Coefficient `json:"coefficients"`
}

func (left Spectrum) Merge(right Merger) (Merger, error) {
	return left, nil
}

type Vector3D struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

func Decode(input interface{}) (interface{}, error) {
	if input == nil {
		// a JSON null, e.g. a cleared Signal K notification, must stay nil:
		// mapstructure.DecodeMetadata trivially "succeeds" decoding a nil
		// input into any all-optional struct (leaving it at its zero value,
		// with no Unused fields to report), so without this check a null
		// would silently be mistaken for the first candidate type below.
		return nil, nil
	}
	if i, ok := input.(int64); ok {
		return i, nil
	}
	if f, ok := input.(float64); ok {
		return f, nil
	}
	if s, ok := input.(string); ok {
		return s, nil
	}

	var metadata mapstructure.Metadata
	p := Position{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &p, &metadata); err == nil && len(metadata.Unused) == 0 {
		return p, nil
	}

	vi := VesselInfo{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &vi, &metadata); err == nil && len(metadata.Unused) == 0 {
		return vi, nil
	}

	vt := VesselType{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &vt, &metadata); err == nil && len(metadata.Unused) == 0 {
		return vt, nil
	}

	l := Length{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &l, &metadata); err == nil && len(metadata.Unused) == 0 {
		return l, nil
	}

	n := Notification{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &n, &metadata); err == nil && len(metadata.Unused) == 0 {
		return n, nil
	}

	ns := []Notification{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &ns, &metadata); err == nil && len(metadata.Unused) == 0 {
		return ns, nil
	}

	d := Draft{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &d, &metadata); err == nil && len(metadata.Unused) == 0 {
		return d, nil
	}

	c := Current{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &c, &metadata); err == nil && len(metadata.Unused) == 0 {
		return c, nil
	}

	s := Spectrum{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &s, &metadata); err == nil && len(metadata.Unused) == 0 {
		return s, nil
	}

	v := Vector3D{}
	metadata = mapstructure.Metadata{}
	if err := mapstructure.DecodeMetadata(input, &v, &metadata); err == nil && len(metadata.Unused) == 0 {
		return v, nil
	}

	return input, fmt.Errorf("don't know how to decode %v", input)
}
