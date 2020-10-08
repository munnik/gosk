package nmea

import (
	"errors"

	goAIS "github.com/BertoldVdb/go-ais"
	"github.com/martinlindhe/unit"
)

// NavigationStatus retrieves the navigation status from the sentence
type NavigationStatus interface {
	GetNavigationStatus() (uint8, uint32, error)
}

// VesselName retrieves the name of the vessel from the sentence
type VesselName interface {
	GetVesselName() (string, uint32, error)
}

// CallSign retrieves the call sign of the vessel from the sentence
type CallSign interface {
	GetCallSign() (string, uint32, error)
}

// IMONumber retrieves the IMO number of the vessel from the sentence
type IMONumber interface {
	GetIMONumber() (string, uint32, error)
}

// ENINumber retrieves the ENI number of the vessel from the sentence
type ENINumber interface {
	// https://en.wikipedia.org/wiki/ENI_number
	GetENINumber() (string, uint32, error)
}

// VesselDimensions retrieves the length and beam of the vessel from the sentence
type VesselDimensions interface {
	GetVesselDimensions() (float64, float64, uint32, error)
}

// GetNavigationStatus retrieves the navigation status from the sentence
func (s VDMVDO) GetNavigationStatus() (uint8, uint32, error) {
	codec := goAIS.CodecNew(false, false)
	codec.DropSpace = true
	result := codec.DecodePacket(s.Payload)
	if positionReport, ok := result.(goAIS.PositionReport); ok && positionReport.Valid {
		return positionReport.NavigationalStatus, result.GetHeader().UserID, nil
	}
	return 0, 0, errors.New("Sentence is not usable or not valid")
}

// GetVesselName retrieves the name of the vessel from the sentence
func (s VDMVDO) GetVesselName() (string, uint32, error) {
	codec := goAIS.CodecNew(false, false)
	codec.DropSpace = true
	result := codec.DecodePacket(s.Payload)
	if staticDataReport, ok := result.(goAIS.StaticDataReport); ok && staticDataReport.Valid && staticDataReport.ReportA.Valid {
		return staticDataReport.ReportA.Name, result.GetHeader().UserID, nil
	}
	if shipStaticData, ok := result.(goAIS.ShipStaticData); ok && shipStaticData.Valid {
		return shipStaticData.Name, result.GetHeader().UserID, nil
	}
	return "", 0, errors.New("Sentence is not usable or not valid")
}

// GetCallSign retrieves the call sign of the vessel from the sentence
func (s VDMVDO) GetCallSign() (string, uint32, error) {
	codec := goAIS.CodecNew(false, false)
	codec.DropSpace = true
	result := codec.DecodePacket(s.Payload)
	if staticDataReport, ok := result.(goAIS.StaticDataReport); ok && staticDataReport.Valid && staticDataReport.ReportB.Valid {
		return staticDataReport.ReportB.CallSign, result.GetHeader().UserID, nil
	}
	if shipStaticData, ok := result.(goAIS.ShipStaticData); ok && shipStaticData.Valid {
		return shipStaticData.CallSign, result.GetHeader().UserID, nil
	}
	return "", 0, errors.New("Sentence is not usable or not valid")
}

// GetIMONumber retrieves the IMO number of the vessel from the sentence
func (s VDMVDO) GetIMONumber() (string, uint32, error) {
	codec := goAIS.CodecNew(false, false)
	codec.DropSpace = true
	result := codec.DecodePacket(s.Payload)
	if shipStaticData, ok := result.(goAIS.ShipStaticData); ok && shipStaticData.Valid {
		return string(shipStaticData.ImoNumber), result.GetHeader().UserID, nil
	}
	return "", 0, errors.New("Sentence is not usable or not valid")
}

// GetENINumber retrieves the ENI number of the vessel from the sentence
func (s VDMVDO) GetENINumber() (string, uint32, error) {
	codec := goAIS.CodecNew(false, false)
	codec.DropSpace = true
	result := codec.DecodePacket(s.Payload)
	if binaryBroadcastMessage, ok := result.(goAIS.BinaryBroadcastMessage); ok && binaryBroadcastMessage.Valid && binaryBroadcastMessage.ApplicationID.DesignatedAreaCode == 200 && binaryBroadcastMessage.ApplicationID.FunctionIdentifier == 10 {
		eniNumber, err := extractString(binaryBroadcastMessage.BinaryData, 0, 48)
		if err != nil {
			return "", 0, errors.New("Could not extract ENI number from binary data")
		}
		return eniNumber, result.GetHeader().UserID, nil
	}
	return "", 0, errors.New("Sentence is not usable or not valid")
}

// GetVesselDimensions retrieves the length and beam of the vessel from the sentence
func (s VDMVDO) GetVesselDimensions() (float64, float64, uint32, error) {
	codec := goAIS.CodecNew(false, false)
	codec.DropSpace = true
	result := codec.DecodePacket(s.Payload)
	if binaryBroadcastMessage, ok := result.(goAIS.BinaryBroadcastMessage); ok && binaryBroadcastMessage.Valid && binaryBroadcastMessage.ApplicationID.DesignatedAreaCode == 200 && binaryBroadcastMessage.ApplicationID.FunctionIdentifier == 10 {
		length, err := extractNumber(binaryBroadcastMessage.BinaryData, 48, 13)
		if err != nil {
			return 0.0, 0.0, 0, errors.New("Could not extract length from binary data")
		}
		beam, err := extractNumber(binaryBroadcastMessage.BinaryData, 61, 10)
		if err != nil {
			return 0.0, 0.0, 0, errors.New("Could not extract beam from binary data")
		}
		return (unit.Length(length) * unit.Decimeter).Meters(), (unit.Length(beam) * unit.Decimeter).Meters(), result.GetHeader().UserID, nil
	}
	return 0.0, 0.0, 0, errors.New("Sentence is not usable or not valid")
}

func extractNumber(binaryData []byte, offset int, length int) (uint64, error) {
	var result uint64 = 0

	for _, value := range binaryData[offset : offset+length] {
		result <<= 1
		result |= uint64(value)
	}

	return result, nil
}

func extractString(binaryData []byte, offset int, length int) (string, error) {
	if (length)%6 != 0 {
		return "", errors.New("Length must be divisible by 6")
	}
	sixBitCharacters := make([]byte, length/6)
	var position int
	for index, value := range binaryData[offset : offset+length] {
		position = index / 6
		sixBitCharacters[position] <<= 1
		sixBitCharacters[position] |= value
	}
	for index, value := range sixBitCharacters {
		if value < 32 {
			sixBitCharacters[index] = value + 64
		}
	}
	return string(sixBitCharacters), nil
}
