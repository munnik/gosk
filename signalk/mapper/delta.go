package mapper

import (
	"fmt"

	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk"
	"github.com/munnik/gosk/signalk/mapper/nmea"
)

const (
	// NMEA0183Type is used to identify the data as NMEA 0183 data
	NMEA0183Type = "NMEA0183"
	// ModbusType is used to identify the data as Modbus data
	ModbusType = "Modbus"
)

// DeltaFromMessage tries to create a SignalK delta from the provided data
func DeltaFromMessage(m nanomsg.Message) (signalk.Delta, error) {
	switch string(m.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL]) {
	case NMEA0183Type:
		return nmea.DeltaFromNMEA0183(m)
	}
	return signalk.DeltaWithContext{}, fmt.Errorf("Don't know how to handle %s", m.HeaderSegments[nanomsg.HEADERSEGMENTPROTOCOL])
}
