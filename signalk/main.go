package signalk

import (
	"fmt"
	"log"
	"strings"
)

var (
	dataTypeForPath map[string]string = map[string]string{
		"navigation.position": "Position",
		"design.length":       "Length",
	}
)

// remember to run `easyjson -all signalk/main.go` when changing a struct

// Delta as specified in https://signalk.org/specification/1.4.0/doc/data_model.html
type Delta struct {
	Context string   `json:"context,omitempty"`
	Updates []Update `json:"updates"`
}

// Full as specified in https://signalk.org/specification/1.4.0/doc/data_model.html
type Full struct {
	Version string                   `json:"version"`
	Self    string                   `json:"self"`
	Vessels map[string]*VesselValues `json:"vessels"`
}

// VesselValues element of a Full model
type VesselValues map[string]interface{}

// Position is used for position values without altitude
type Position struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	Altitude  float64 `json:"altitude,omitempty"`
}

// Length is used for the combination of all lengths
type Length struct {
	Overall   float64 `json:"overall,omitempty"`
	Hull      float64 `json:"hull,omitempty"`
	Waterline float64 `json:"waterline,omitempty"`
}

// Value is part of an Update
type Value struct {
	Context string      `json:"context,omitempty"`
	Path    []string    `json:"path"`
	Value   interface{} `json:"value"`
}

// Source is part of an Update
type Source struct {
	Label    string `json:"label"`
	Type     string `json:"type"`
	Talker   string `json:"talker,omitempty"`
	Sentence string `json:"sentence,omitempty"`
	AisType  uint8  `json:"aisType,omitempty"`
}

// Update is part of a Delta
type Update struct {
	Source    Source  `json:"source"`
	Timestamp string  `json:"timestamp"`
	Values    []Value `json:"values"`
}

// NewFull creates a Full model
func NewFull(self string) *Full {
	return &Full{
		Version: "1.0.0",
		Self:    self,
		Vessels: make(map[string]*VesselValues),
	}
}

// AddValue adds a value to the Full model
func (full *Full) AddValue(value Value) {
	objectType := strings.Split(value.Context, ".")[0]
	context := value.Context[len(objectType)+1:]
	switch objectType {
	case "vessels":
		if _, exists := full.Vessels[context]; !exists {
			full.Vessels[context] = &VesselValues{}
		}
		value.Context = ""
		full.Vessels[context].AddValue(value)
	}
}

// AddValue adds a value to the Vessel
func (vesselValues *VesselValues) AddValue(value Value) {
	if len(value.Path) == 0 {
		return
	}
	if len(value.Path) == 1 {
		(*vesselValues)[value.Path[0]] = &value.Value
		return
	}

	currentPath := value.Path[0]
	value.Path = value.Path[1:]

	if _, exist := (*vesselValues)[currentPath]; !exist {
		(*vesselValues)[currentPath] = &VesselValues{}
	}
	(*vesselValues)[currentPath].(*VesselValues).AddValue(value)
}

// FromJSONToStruct tries convert a value to a struct, it lookups the path of the value
// in a dictionary to find the corresponding data type. Then it tries to Unmarshal the
// value to that data type.
func FromJSONToStruct(value string, path string) (interface{}, error) {
	typeOfValue, ok := dataTypeForPath[path]
	if !ok {
		return nil, fmt.Errorf("Lookup of the path %s failed", path)
	}
	switch typeOfValue {
	case "Position":
		var position Position
		err := position.UnmarshalJSON([]byte(value))
		if err != nil {
			log.Fatal(err)
		}
		return position, nil
	case "Length":
		var length Length
		err := length.UnmarshalJSON([]byte(value))
		if err != nil {
			log.Fatal(err)
		}
		return length, nil
	}
	return nil, fmt.Errorf("Not defined how to unmarshal %s", typeOfValue)
}

// var delta signalk.Delta
// if v, ok := sentence.(VDMVDO); ok && v.Packet != nil {
// 	delta = signalk.DeltaWithContext{
// 		Context: fmt.Sprintf("vessels.urn:mrn:imo:mmsi:%d", v.Packet.GetHeader().UserID),
// 		Updates: []signalk.Update{
// 			{
// 				Source: signalk.Source{
// 					Label:    string(m.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE]),
// 					Type:     string(m.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL]),
// 					Sentence: sentence.DataType(),
// 					Talker:   sentence.TalkerID(),
// 					AisType:  v.Packet.GetHeader().MessageID,
// 				},
// 				Timestamp: m.Time.UTC().Format(time.RFC3339),
// 			},
// 		},
// 	}
// } else {
// 	delta = signalk.DeltaWithoutContext{
// 		Updates: []signalk.Update{
// 			{
// 				Source: signalk.Source{
// 					Label:    string(m.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE]),
// 					Type:     string(m.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL]),
// 					Sentence: sentence.DataType(),
// 					Talker:   sentence.TalkerID(),
// 				}, Timestamp: m.Time.UTC().Format(time.RFC3339),
// 			},
// 		},
// 	}
// }
