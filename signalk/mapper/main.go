package mapper

import (
	"fmt"
	"log"

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
func KeyValueFromData(m *nanomsg.Message) ([]signalk.Value, error) {
	switch string(m.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL]) {
	case NMEA0183Type:
		return nmea.KeyValueFromNMEA0183(m)
	}
	return nil, fmt.Errorf("Don't know how to handle %s", m.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL])
}

// Map raw messages to key value messages
func Map(subscriber mangos.Socket, publisher mangos.Socket) {
	for {
		raw, err := subscriber.Recv()
		if err != nil {
			log.Fatal(err)
		}
		rawMessage, err := nanomsg.Parse(raw)
		if err != nil {
			log.Fatal(err)
		}
		values, err := KeyValueFromData(rawMessage)
		if err != nil {
			log.Println(err)
		}
		for _, value := range values {
			json, err := value.MarshalJSON()
			if err != nil {
				log.Fatal(err)
			}
			mappedHeader := [][]byte{[]byte("mapper")}
			mappedHeader = append(mappedHeader, rawMessage.HeaderSegments[1:]...)
			mappedMessage := nanomsg.NewMessage(json, rawMessage.Time, mappedHeader...)
			publisher.Send([]byte(mappedMessage.String()))
		}
	}
}
