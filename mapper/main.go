package mapper

import (
	"strings"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper/modbus"
	"github.com/munnik/gosk/mapper/nmea0183"
	"github.com/munnik/gosk/mapper/signalk"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type WrongMapperError struct {
	ExpectedSource string
	FoundSource    string
}

func (e *WrongMapperError) Error() string {
	return "Wrong protocol or source name does not match"
}

// KeyValueFromData tries to create a SignalK delta from the provided data
func KeyValueFromData(m *nanomsg.RawData, cfg interface{}) ([]signalk.Value, error) {
	switch string(m.Header.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL]) {
	case config.NMEA0183Type:
		if _, ok := cfg.(config.NMEA0183Config); ok && cfg.(config.NMEA0183Config).Name == m.Header.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE] {
			return nmea0183.KeyValueFromNMEA0183(m, cfg.(config.NMEA0183Config))
		}
	case config.ModbusType:
		if _, ok := cfg.(config.ModbusConfig); ok && cfg.(config.ModbusConfig).Name == m.Header.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE] {
			return modbus.KeyValueFromModbus(m, cfg.(config.ModbusConfig))
		}
	}
	return nil, &WrongMapperError{}
}

// Map raw messages to key value messages
func Map(subscriber mangos.Socket, publisher mangos.Socket, cfg interface{}) {
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
		values, err := KeyValueFromData(rawData, cfg)
		if err != nil {
			if _, ok := err.(*WrongMapperError); !ok {
				logger.GetLogger().Warn(
					"Could not extract values from the raw data",
					zap.ByteString("Raw data", rawData.Payload),
					zap.String("Error", err.Error()),
				)
			}
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
			} else if v, ok := value.Value.(nanomsg.VesselData); ok {
				mappedData = &nanomsg.MappedData{
					Header: &nanomsg.Header{
						HeaderSegments: append([]string{"mapper"}, rawData.Header.HeaderSegments[1:]...),
					},
					Timestamp:       rawData.Timestamp,
					Context:         value.Context,
					Path:            strings.Join(value.Path, "."),
					Datatype:        nanomsg.VESSELDATA,
					VesselDataValue: &v,
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
