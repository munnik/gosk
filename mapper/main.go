package mapper

import (
	"fmt"
	"strings"

	"github.com/munnik/gosk/maper/signalk"
	"github.com/munnik/gosk/mapper/nmea0183"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos"
	"google.golang.org/protobuf/proto"
)

const (
	// NMEA0183Type is used to identify the data as NMEA 0183 data
	NMEA0183Type = "NMEA0183"
	// ModbusType is used to identify the data as Modbus data
	ModbusType = "Modbus"
)

// KeyValueFromData tries to create a SignalK delta from the provided data
func KeyValueFromData(m *nanomsg.RawData) ([]signalk.Value, error) {
	switch string(m.Header.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL]) {
	case NMEA0183Type:
		return nmea0183.KeyValueFromNMEA0183(m)
	}
	return nil, fmt.Errorf("Don't know how to handle %s", m.Header.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL])
}

// Map raw messages to key value messages
func Map(subscriber mangos.Socket, publisher mangos.Socket) {
	rawData := &nanomsg.RawData{}
	for {
		received, err := subscriber.Recv()
		if err != nil {
			log.Warn(err)
		}
		if err := proto.Unmarshal(received, rawData); err != nil {
			log.Warn(err)
		}
		values, err := KeyValueFromData(rawData)
		if err != nil {
			log.Warn(err)
		}
		for _, value := range values {
			var mappedData *nanomsg.MappedData
			mappedData = nil
			if v, ok := value.Value.(float64); ok {
				mappedData = &nanomsg.MappedData{
					Header: &nanomsg.Header{
						HeaderSegments: append([]string{"mapper"}, rawData.Header.HeaderSegments[1:]...),
					},
					Timestamp:   rawData.Timestamp,
					Context:     value.Context,
					Path:        strings.Join(value.Path, "."),
					Datatype:    nanomsg.DOUBLE,
					DoubleValue: v,
				}
			} else if v, ok := value.Value.(string); ok {
				mappedData = &nanomsg.MappedData{
					Header: &nanomsg.Header{
						HeaderSegments: append([]string{"mapper"}, rawData.Header.HeaderSegments[1:]...),
					},
					Timestamp:   rawData.Timestamp,
					Context:     value.Context,
					Path:        strings.Join(value.Path, "."),
					Datatype:    nanomsg.STRING,
					StringValue: v,
				}
			} else if v, ok := value.Value.(nanomsg.Position); ok {
				mappedData = &nanomsg.MappedData{
					Header: &nanomsg.Header{
						HeaderSegments: append([]string{"mapper"}, rawData.Header.HeaderSegments[1:]...),
					},
					Timestamp:     rawData.Timestamp,
					Context:       value.Context,
					Path:          strings.Join(value.Path, "."),
					Datatype:      nanomsg.POSITION,
					PositionValue: &v,
				}
			} else if v, ok := value.Value.(nanomsg.Length); ok {
				mappedData = &nanomsg.MappedData{
					Header: &nanomsg.Header{
						HeaderSegments: append([]string{"mapper"}, rawData.Header.HeaderSegments[1:]...),
					},
					Timestamp:   rawData.Timestamp,
					Context:     value.Context,
					Path:        strings.Join(value.Path, "."),
					Datatype:    nanomsg.LENGTH,
					LengthValue: &v,
				}
			} else {
				continue
			}

			toSend, err := proto.Marshal(mappedData)
			if err != nil {
				log.Fatal(err)
			}
			publisher.Send(toSend)
		}
	}
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
