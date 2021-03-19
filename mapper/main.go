package mapper

import (
	"fmt"
	"strings"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper/modbus"
	"github.com/munnik/gosk/mapper/nmea0183"
	"github.com/munnik/gosk/mapper/signalk"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
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
	case ModbusType:
		config := modbus.MappingConfig{
			RegisterMappings: map[uint16]modbus.RegisterMapping{
				uint16(51300): {
					Size:            2,
					MappingFunction: "(registers[0] * 65536 + registers[1]) / 60000.0",
					SignalKPath:     []string{"propulsion", "mainEngine", "revolutions"},
				},
				uint16(51460): {
					Size:            2,
					MappingFunction: "273.15 + (registers[0] * 65536 + registers[1]) / 1000.0",
					SignalKPath:     []string{"propulsion", "mainEngine", "oilTemperature"},
				},
			},
			Context: "vessels.urn:mrn:imo:mmsi:244770688",
		}
		return modbus.KeyValueFromModbus(m, config)
	}
	return nil, fmt.Errorf("don't know how to handle %s", m.Header.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL])
}

// Map raw messages to key value messages
func Map(subscriber mangos.Socket, publisher mangos.Socket) {
	rawData := &nanomsg.RawData{}
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := proto.Unmarshal(received, rawData); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		values, err := KeyValueFromData(rawData)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not extract values from the raw data",
				zap.ByteString("Raw data", rawData.Payload),
				zap.String("Error", err.Error()),
			)
			continue
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
				logger.GetLogger().Warn(
					"Could not marshall the mapped data",
					zap.String("Mapped data", mappedData.String()),
					zap.String("Error", err.Error()),
				)
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
