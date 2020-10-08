package nmea

import (
	"errors"

	goAIS "github.com/BertoldVdb/go-ais"
	"github.com/martinlindhe/unit"
)

// NavigationStatus retrieves the navigation status from the sentence
type NavigationStatus interface {
	GetNavigationStatus() (uint8, error)
}

// VesselName retrieves the name of the vessel from the sentence
type VesselName interface {
	GetVesselName() (string, error)
}

// CallSign retrieves the call sign of the vessel from the sentence
type CallSign interface {
	GetCallSign() (string, error)
}

// IMONumber retrieves the IMO number of the vessel from the sentence
type IMONumber interface {
	GetIMONumber() (string, error)
}

// ENINumber retrieves the ENI number of the vessel from the sentence
type ENINumber interface {
	// https://en.wikipedia.org/wiki/ENI_number
	GetENINumber() (string, error)
}

// VesselDimensions retrieves the length and beam of the vessel from the sentence
type VesselDimensions interface {
	GetVesselDimensions() (float64, float64, error)
}

// GetNavigationStatus retrieves the navigation status from the sentence
func (s VDMVDO) GetNavigationStatus() (uint8, error) {
	if positionReport, ok := s.Packet.(goAIS.PositionReport); ok && positionReport.Valid {
		return positionReport.NavigationalStatus, nil
	}
	return 0, errors.New("Sentence is not usable or not valid")
}

// GetVesselName retrieves the name of the vessel from the sentence
func (s VDMVDO) GetVesselName() (string, error) {
	if staticDataReport, ok := s.Packet.(goAIS.StaticDataReport); ok && staticDataReport.Valid && staticDataReport.ReportA.Valid {
		return staticDataReport.ReportA.Name, nil
	}
	if shipStaticData, ok := s.Packet.(goAIS.ShipStaticData); ok && shipStaticData.Valid {
		return shipStaticData.Name, nil
	}
	return "", errors.New("Sentence is not usable or not valid")
}

// GetCallSign retrieves the call sign of the vessel from the sentence
func (s VDMVDO) GetCallSign() (string, error) {
	if staticDataReport, ok := s.Packet.(goAIS.StaticDataReport); ok && staticDataReport.Valid && staticDataReport.ReportB.Valid {
		return staticDataReport.ReportB.CallSign, nil
	}
	if shipStaticData, ok := s.Packet.(goAIS.ShipStaticData); ok && shipStaticData.Valid {
		return shipStaticData.CallSign, nil
	}
	return "", errors.New("Sentence is not usable or not valid")
}

// GetIMONumber retrieves the IMO number of the vessel from the sentence
func (s VDMVDO) GetIMONumber() (string, error) {
	if shipStaticData, ok := s.Packet.(goAIS.ShipStaticData); ok && shipStaticData.Valid {
		return string(shipStaticData.ImoNumber), nil
	}
	return "", errors.New("Sentence is not usable or not valid")
}

// GetENINumber retrieves the ENI number of the vessel from the sentence
func (s VDMVDO) GetENINumber() (string, error) {
	if binaryBroadcastMessage, ok := s.Packet.(goAIS.BinaryBroadcastMessage); ok && binaryBroadcastMessage.Valid && binaryBroadcastMessage.ApplicationID.DesignatedAreaCode == 200 && binaryBroadcastMessage.ApplicationID.FunctionIdentifier == 10 {
		eniNumber, err := extractString(binaryBroadcastMessage.BinaryData, 0, 48)
		if err != nil {
			return "", errors.New("Could not extract ENI number from binary data")
		}
		return eniNumber, nil
	}
	return "", errors.New("Sentence is not usable or not valid")
}

// GetVesselDimensions retrieves the length and beam of the vessel from the sentence
func (s VDMVDO) GetVesselDimensions() (float64, float64, error) {
	if binaryBroadcastMessage, ok := s.Packet.(goAIS.BinaryBroadcastMessage); ok && binaryBroadcastMessage.Valid && binaryBroadcastMessage.ApplicationID.DesignatedAreaCode == 200 && binaryBroadcastMessage.ApplicationID.FunctionIdentifier == 10 {
		length, err := extractNumber(binaryBroadcastMessage.BinaryData, 48, 13)
		if err != nil {
			return 0.0, 0.0, errors.New("Could not extract length from binary data")
		}
		beam, err := extractNumber(binaryBroadcastMessage.BinaryData, 61, 10)
		if err != nil {
			return 0.0, 0.0, errors.New("Could not extract beam from binary data")
		}
		return (unit.Length(length) * unit.Decimeter).Meters(), (unit.Length(beam) * unit.Decimeter).Meters(), nil
	}
	return 0.0, 0.0, errors.New("Sentence is not usable or not valid")
}
