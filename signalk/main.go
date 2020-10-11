package signalk

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
	Context string      `json:"context"`
	Path    string      `json:"path"`
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
