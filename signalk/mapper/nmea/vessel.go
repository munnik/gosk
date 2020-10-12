package nmea

import (
	"errors"
	"fmt"

	goAIS "github.com/BertoldVdb/go-ais"
	"github.com/martinlindhe/unit"
)

// CallSign retrieves the call sign of the vessel from the sentence
type CallSign interface {
	GetCallSign() (string, error)
}

// ENINumber retrieves the ENI number of the vessel from the sentence
type ENINumber interface {
	// https://en.wikipedia.org/wiki/ENI_number
	GetENINumber() (string, error)
}

// IMONumber retrieves the IMO number of the vessel from the sentence
type IMONumber interface {
	GetIMONumber() (string, error)
}

// MMSI retrieves the MMSI of the vessel from the sentence
type MMSI interface {
	GetMMSI() (uint32, error)
}

// NavigationStatus retrieves the navigation status from the sentence
type NavigationStatus interface {
	GetNavigationStatus() (string, error)
}

// VesselLength retrieves the length of the vessel from the sentence
type VesselLength interface {
	GetVesselLength() (float64, error)
}

// VesselBeam retrieves the beam of the vessel from the sentence
type VesselBeam interface {
	GetVesselBeam() (float64, error)
}

// VesselName retrieves the name of the vessel from the sentence
type VesselName interface {
	GetVesselName() (string, error)
}

// VesselType retrieves the type of the vessel from the sentence
type VesselType interface {
	GetVesselType() (string, error)
}

var navigationStatuses []string = []string{
	"motoring",
	"anchored",
	"not under command",
	"restricted manouverability",
	"constrained by draft",
	"moored",
	"aground",
	"fishing",
	"sailing",
	"hazardous material high speed",
	"hazardous material wing in ground",
	"reserved for future use",
	"reserved for future use",
	"reserved for future use",
	"ais-sart",
	"default",
}

var vesselTypes []string = []string{
	"Not available (default)",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Reserved for future use",
	"Wing in ground (WIG), all ships of this type",
	"Wing in ground (WIG), Hazardous category A",
	"Wing in ground (WIG), Hazardous category B",
	"Wing in ground (WIG), Hazardous category C",
	"Wing in ground (WIG), Hazardous category D",
	"Wing in ground (WIG), Reserved for future use",
	"Wing in ground (WIG), Reserved for future use",
	"Wing in ground (WIG), Reserved for future use",
	"Wing in ground (WIG), Reserved for future use",
	"Wing in ground (WIG), Reserved for future use",
	"Fishing",
	"Towing",
	"Towing: length exceeds 200m or breadth exceeds 25m",
	"Dredging or underwater ops",
	"Diving ops",
	"Military ops",
	"Sailing",
	"Pleasure Craft",
	"Reserved",
	"Reserved",
	"High speed craft (HSC), all ships of this type",
	"High speed craft (HSC), Hazardous category A",
	"High speed craft (HSC), Hazardous category B",
	"High speed craft (HSC), Hazardous category C",
	"High speed craft (HSC), Hazardous category D",
	"High speed craft (HSC), Reserved for future use",
	"High speed craft (HSC), Reserved for future use",
	"High speed craft (HSC), Reserved for future use",
	"High speed craft (HSC), Reserved for future use",
	"High speed craft (HSC), No additional information",
	"Pilot Vessel",
	"Search and Rescue vessel",
	"Tug",
	"Port Tender",
	"Anti-pollution equipment",
	"Law Enforcement",
	"Spare - Local Vessel",
	"Spare - Local Vessel",
	"Medical Transport",
	"Noncombatant ship according to RR Resolution No. 18",
	"Passenger, all ships of this type",
	"Passenger, Hazardous category A",
	"Passenger, Hazardous category B",
	"Passenger, Hazardous category C",
	"Passenger, Hazardous category D",
	"Passenger, Reserved for future use",
	"Passenger, Reserved for future use",
	"Passenger, Reserved for future use",
	"Passenger, Reserved for future use",
	"Passenger, No additional information",
	"Cargo, all ships of this type",
	"Cargo, Hazardous category A",
	"Cargo, Hazardous category B",
	"Cargo, Hazardous category C",
	"Cargo, Hazardous category D",
	"Cargo, Reserved for future use",
	"Cargo, Reserved for future use",
	"Cargo, Reserved for future use",
	"Cargo, Reserved for future use",
	"Cargo, No additional information",
	"Tanker, all ships of this type",
	"Tanker, Hazardous category A",
	"Tanker, Hazardous category B",
	"Tanker, Hazardous category C",
	"Tanker, Hazardous category D",
	"Tanker, Reserved for future use",
	"Tanker, Reserved for future use",
	"Tanker, Reserved for future use",
	"Tanker, Reserved for future use",
	"Tanker, No additional information",
	"Other Type, all ships of this type",
	"Other Type, Hazardous category A",
	"Other Type, Hazardous category B",
	"Other Type, Hazardous category C",
	"Other Type, Hazardous category D",
	"Other Type, Reserved for future use",
	"Other Type, Reserved for future use",
	"Other Type, Reserved for future use",
	"Other Type, Reserved for future use",
	"Other Type, no additional information",
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

// GetIMONumber retrieves the IMO number of the vessel from the sentence
func (s VDMVDO) GetIMONumber() (string, error) {
	if shipStaticData, ok := s.Packet.(goAIS.ShipStaticData); ok && shipStaticData.Valid {
		return fmt.Sprintf("%d", shipStaticData.ImoNumber), nil
	}
	return "", errors.New("Sentence is not usable or not valid")
}

// GetMMSI retrieves the MMSI of the vessel from the sentence
func (s VDMVDO) GetMMSI() (uint32, error) {
	if s.Packet == nil || s.Packet.GetHeader() == nil {
		return 0, errors.New("Could not retrieve MMSI")
	}
	return s.Packet.GetHeader().UserID, nil
}

// GetNavigationStatus retrieves the navigation status from the sentence
func (s VDMVDO) GetNavigationStatus() (string, error) {
	if positionReport, ok := s.Packet.(goAIS.PositionReport); ok && positionReport.Valid {
		return navigationStatuses[positionReport.NavigationalStatus], nil
	}
	return navigationStatuses[15], errors.New("Sentence is not usable or not valid")
}

// GetVesselBeam retrieves the beam of the vessel from the sentence
func (s VDMVDO) GetVesselBeam() (float64, error) {
	if binaryBroadcastMessage, ok := s.Packet.(goAIS.BinaryBroadcastMessage); ok && binaryBroadcastMessage.Valid && binaryBroadcastMessage.ApplicationID.DesignatedAreaCode == 200 && binaryBroadcastMessage.ApplicationID.FunctionIdentifier == 10 {
		beam, err := extractNumber(binaryBroadcastMessage.BinaryData, 61, 10)
		if err != nil {
			return 0.0, errors.New("Could not extract beam from binary data")
		}
		return (unit.Length(beam) * unit.Decimeter).Meters(), nil
	}
	return 0.0, errors.New("Sentence is not usable or not valid")
}

// GetVesselLength retrieves the length of the vessel from the sentence
func (s VDMVDO) GetVesselLength() (float64, error) {
	if binaryBroadcastMessage, ok := s.Packet.(goAIS.BinaryBroadcastMessage); ok && binaryBroadcastMessage.Valid && binaryBroadcastMessage.ApplicationID.DesignatedAreaCode == 200 && binaryBroadcastMessage.ApplicationID.FunctionIdentifier == 10 {
		length, err := extractNumber(binaryBroadcastMessage.BinaryData, 48, 13)
		if err != nil {
			return 0.0, errors.New("Could not extract length from binary data")
		}
		return (unit.Length(length) * unit.Decimeter).Meters(), nil
	}
	return 0.0, errors.New("Sentence is not usable or not valid")
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

// GetVesselType retrieves the type of the vessel from the sentence
func (s VDMVDO) GetVesselType() (string, error) {
	var vesselTypeIndex uint8
	if staticDataReport, ok := s.Packet.(goAIS.StaticDataReport); ok && staticDataReport.Valid && staticDataReport.ReportB.Valid {
		vesselTypeIndex = staticDataReport.ReportB.ShipType
	} else if shipStaticData, ok := s.Packet.(goAIS.ShipStaticData); ok && shipStaticData.Valid {
		vesselTypeIndex = shipStaticData.Type
	} else {
		return vesselTypes[0], errors.New("Sentence is not usable or not valid")
	}
	if vesselTypeIndex < uint8(len(vesselTypes)) {
		return vesselTypes[vesselTypeIndex], nil
	}
	return vesselTypes[0], errors.New("Unknown vessel type")
}
