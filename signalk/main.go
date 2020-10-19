package signalk

// remember to run `easyjson -all signalk/main.go` when changing a struct

// Delta as specified in https://signalk.org/specification/1.4.0/doc/data_model.html
type Delta struct {
	Context string   `json:"context,omitempty"`
	Updates []Update `json:"updates"`
}

// Full as specified in https://signalk.org/specification/1.4.0/doc/data_model.html
type Full struct {
	Version string            `json:"version"`
	Self    string            `json:"self"`
	Vessels map[string]Vessel `json:"vessels"`
}

// Vessel element of a Full model
type Vessel struct {
	elements map[string]Value
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
		Vessels: make(map[string]Vessel, 0),
	}
}

// AddValue adds a value to the Full model
func (full *Full) AddValue(value Value) {
	if _, exists := full.Vessels[value.Context]; !exists {
		full.Vessels[value.Context] = Vessel{}
	}
	vessel := full.Vessels[value.Context]
	vessel.AddValue(value)
}

func (vessel *Vessel) AddValue(value Value) {

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
