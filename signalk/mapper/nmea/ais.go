package nmea

import (
	"errors"

	goAIS "github.com/BertoldVdb/go-ais"
)

// NavigationStatus retrieves the navigation status from the sentence
type NavigationStatus interface {
	GetNavigationStatus() (uint8, uint32, error)
}

// GetNavigationStatus retrieves the navigation status from the sentence
func (s VDMVDO) GetNavigationStatus() (uint8, uint32, error) {
	codec := goAIS.CodecNew(false, false)
	result := codec.DecodePacket(s.Payload)
	if positionReport, ok := result.(goAIS.PositionReport); ok {
		return positionReport.NavigationalStatus, result.GetHeader().UserID, nil
	}
	return 0, 0, errors.New("Not a position report")
}
