package mapper

import (
	"fmt"
	"log"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk"
	"github.com/munnik/gosk/signalk/mapper/nmea"
	"go.nanomsg.org/mangos/v3"
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
		return nmea.KeyValueFromNMEA0183(m)
	}
	return nil, fmt.Errorf("Don't know how to handle %s", m.Header.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL])
}

// Map raw messages to key value messages
func Map(subscriber mangos.Socket, publisher mangos.Socket) {
	rawData := &nanomsg.RawData{}
	for {
		received, err := subscriber.Recv()
		if err != nil {
			log.Fatal(err)
		}
		if err := proto.Unmarshal(received, rawData); err != nil {
			log.Fatal(err)
		}
		values, err := KeyValueFromData(rawData)
		if err != nil {
			log.Println(err)
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
