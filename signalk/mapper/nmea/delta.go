package nmea

import (
	"fmt"
	"time"

	"github.com/munnik/gosk/signalk"
)

// DeltaFromNMEA0183 tries to create a Signal K Delta from a NMEA sentence
func DeltaFromNMEA0183(line []byte, collectorName string) (signalk.Delta, error) {
	sentence, err := Parse(string(line))
	if err != nil {
		return &signalk.DeltaWithoutContext{}, err
	}
	var delta signalk.Delta
	if v, ok := sentence.(VDMVDO); ok && v.Packet != nil {
		delta = signalk.DeltaWithContext{
			Context: fmt.Sprintf("vessels.urn:mrn:imo:mmsi:%d", v.Packet.GetHeader().UserID),
			Updates: []signalk.Update{
				{
					Source: signalk.Source{
						Label:    collectorName,
						Type:     "NMEA0183",
						Sentence: sentence.DataType(),
						Talker:   sentence.TalkerID(),
						AisType:  v.Packet.GetHeader().MessageID,
					},
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				},
			},
		}
	} else {
		delta = signalk.DeltaWithoutContext{
			Updates: []signalk.Update{
				{
					Source: signalk.Source{
						Label:    collectorName,
						Type:     "NMEA0183",
						Sentence: sentence.DataType(),
						Talker:   sentence.TalkerID(),
					}, Timestamp: time.Now().UTC().Format(time.RFC3339),
				},
			},
		}
	}

	if v, ok := sentence.(DepthBelowSurface); ok {
		depthBelowSurface, err := v.GetDepthBelowSurface()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "environment/depth/belowSurface", Value: depthBelowSurface},
			)
		}
	}
	if v, ok := sentence.(DepthBelowTransducer); ok {
		depthBelowTransducer, err := v.GetDepthBelowTransducer()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "environment/depth/belowTransducer", Value: depthBelowTransducer},
			)
		}
	}
	if v, ok := sentence.(FixQuality); ok {
		fixQuality, err := v.GetFixQuality()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/gnss/methodQuality", Value: fixQuality},
			)
		}
	}
	if v, ok := sentence.(FixType); ok {
		fixType, err := v.GetFixType()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/gnss/type", Value: fixType},
			)
		}
	}
	if v, ok := sentence.(RateOfTurn); ok {
		rateOfTurn, err := v.GetRateOfTurn()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/rateOfTurn", Value: rateOfTurn},
			)
		}
	}
	if v, ok := sentence.(TrueCourseOverGround); ok {
		trueCourseOverGround, err := v.GetTrueCourseOverGround()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/courseOverGroundTrue", Value: trueCourseOverGround},
			)
		}
	}
	if v, ok := sentence.(TrueHeading); ok {
		trueHeading, err := v.GetTrueHeading()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/headingTrue", Value: trueHeading},
			)
		}
	}
	if v, ok := sentence.(MagneticHeading); ok {
		magneticHeading, err := v.GetMagneticHeading()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/headingMagnetic", Value: magneticHeading},
			)
		}
	}
	if v, ok := sentence.(NavigationStatus); ok {
		navigationStatus, err := v.GetNavigationStatus()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/state", Value: navigationStatus},
			)
		}
	}
	if v, ok := sentence.(NumberOfSatelites); ok {
		numberOfSatelites, err := v.GetNumberOfSatelites()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/gnss/satellites", Value: numberOfSatelites},
			)
		}
	}
	if v, ok := sentence.(Position2D); ok {
		lon, lat, err := v.GetPosition2D()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/position", Value: signalk.Position2D{Longitude: lon, Latitude: lat}},
			)
		}
	}
	if v, ok := sentence.(Position3D); ok {
		lon, lat, alt, err := v.GetPosition3D()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/position", Value: signalk.Position3D{Position2D: signalk.Position2D{Longitude: lon, Latitude: lat}, Altitude: alt}},
			)
		}
	}
	if v, ok := sentence.(SpeedOverGround); ok {
		speedOverGround, err := v.GetSpeedOverGround()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/speedOverGround", Value: speedOverGround},
			)
		}
	}
	if v, ok := sentence.(SpeedThroughWater); ok {
		speedThroughWater, err := v.GetSpeedThroughWater()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "navigation/speedThroughWater", Value: speedThroughWater},
			)
		}
	}
	if v, ok := sentence.(VesselName); ok {
		vesselName, err := v.GetVesselName()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "name", Value: vesselName},
			)
		}
	}
	if v, ok := sentence.(CallSign); ok {
		callSign, err := v.GetCallSign()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "communication/callsignVhf", Value: callSign},
			)
		}
	}
	if v, ok := sentence.(IMONumber); ok {
		imoNumber, err := v.GetIMONumber()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "registrations/imo", Value: imoNumber},
			)
		}
	}
	if v, ok := sentence.(ENINumber); ok {
		eniNumber, err := v.GetENINumber()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "registrations/other/eni/registration", Value: eniNumber},
			)
		}
	}
	if v, ok := sentence.(VesselDimensions); ok {
		length, beam, err := v.GetVesselDimensions()
		if err == nil {
			delta.AppendValue(
				signalk.Value{Path: "design/length", Value: signalk.OverallLength{Overall: length}},
			)
			delta.AppendValue(
				signalk.Value{Path: "design/beam", Value: beam},
			)
		}
	}

	// if len(delta.Values) == 0 {
	// 	return delta, fmt.Errorf("Could not extract information from the sentence: %s", sentence)
	// }
	return delta, nil
}
