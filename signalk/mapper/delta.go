package mapper

import (
	"fmt"

	"github.com/munnik/gosk/signalk"
	"github.com/munnik/gosk/signalk/mapper/nmea"
)

const (
	// NMEA0183Type is used to identify the data as NMEA 0183 data
	NMEA0183Type = "NMEA0183"
	// ModbusType is used to identify the data as Modbus data
	ModbusType = "Modbus"
)

// DeltaFromData tries to create a SignalK delta from the provided data
func DeltaFromData(data []byte, dataType string, collectorName string) (signalk.Delta, error) {
	switch dataType {
	case NMEA0183Type:
		return nmea.DeltaFromNMEA0183(data, collectorName)
	}
	return signalk.DeltaWithContext{}, fmt.Errorf("Don't know how to handle %s", dataType)
}
