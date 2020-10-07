package mapper

import (
	"fmt"

	"github.com/munnik/gosk/signalk"
	"github.com/munnik/gosk/signalk/mapper/nmea"
)

const (
	// NMEAType is used to identify the data as NMEA data
	NMEAType = "NMEA"
	// ModbusType is used to identify the data as Modbus data
	ModbusType = "Modbus"
)

// DeltaFromData tries to create a SignalK delta from the provided data
func DeltaFromData(data []byte, dataType string) (signalk.Delta, error) {
	switch dataType {
	case NMEAType:
		return nmea.DeltaFromNMEA(data)
	}
	return signalk.Delta{}, fmt.Errorf("Don't know how to handle %s", dataType)
}
