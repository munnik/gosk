package mapper

import (
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper/modbus"
	"github.com/munnik/gosk/mapper/nmea"
	"github.com/munnik/gosk/mapper/signalk"
	"github.com/munnik/gosk/mapper/sygo"
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
	logger.GetLogger().Warn(
		"Received raw data with protocol",
		zap.String("Protocol", m.Header.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL]),
		zap.String("Source", m.Header.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE]),
		zap.ByteString("Bytes", m.Payload),
	)
	switch string(m.Header.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL]) {
	case config.NMEA0183Type:
		if c, ok := cfg.(config.NMEA0183Config); ok && cfg.(config.NMEA0183Config).Name == m.Header.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE] {
			return nmea.KeyValueFromNMEA0183(m, c)
		}
	case config.ModbusType:
		if c, ok := cfg.(config.ModbusConfig); ok && cfg.(config.ModbusConfig).Name == m.Header.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE] {
			return modbus.KeyValueFromModbus(m, c)
		}
	case config.SygoType:
		if c, ok := cfg.(config.SygoConfig); ok && cfg.(config.SygoConfig).Name == m.Header.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE] {
			return sygo.KeyValueFromSygo(m, c)
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
			if v, ok := value.Value.(nanomsg.MappedDataCreator); ok {
				mappedData = v.CreateMappedData(rawData, value.Context, value.Path)
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
