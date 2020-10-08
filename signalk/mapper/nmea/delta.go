package nmea

import (
	"time"

	"github.com/munnik/gosk/signalk"
)

// DeltaFromNMEA tries to create a Signal K Delta from a NMEA sentence
func DeltaFromNMEA(line []byte) (signalk.Delta, error) {
	var delta signalk.Delta
	sentence, err := Parse(string(line))
	if err != nil {
		return delta, err
	}
	delta.Context = "self"
	delta.Updates = append(
		delta.Updates,
		signalk.Update{Source: sentence.TalkerID(), Timestamp: time.Now().UTC().Format(time.RFC3339)},
	)

	if v, ok := sentence.(DepthBelowSurface); ok {
		depthBelowSurface, _, err := v.GetDepthBelowSurface()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "environment/depth/belowSurface", Value: depthBelowSurface},
			)
		}
	}
	if v, ok := sentence.(DepthBelowTransducer); ok {
		depthBelowTransducer, _, err := v.GetDepthBelowTransducer()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "environment/depth/belowTransducer", Value: depthBelowTransducer},
			)
		}
	}
	if v, ok := sentence.(FixQuality); ok {
		fixQuality, _, err := v.GetFixQuality()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/gnss/methodQuality", Value: fixQuality},
			)
		}
	}
	if v, ok := sentence.(FixType); ok {
		fixType, _, err := v.GetFixType()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/gnss/type", Value: fixType},
			)
		}
	}
	if v, ok := sentence.(RateOfTurn); ok {
		rateOfTurn, _, err := v.GetRateOfTurn()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/rateOfTurn", Value: rateOfTurn},
			)
		}
	}
	if v, ok := sentence.(TrueCourseOverGround); ok {
		trueCourseOverGround, _, err := v.GetTrueCourseOverGround()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/courseOverGroundTrue", Value: trueCourseOverGround},
			)
		}
	}
	if v, ok := sentence.(TrueHeading); ok {
		trueHeading, _, err := v.GetTrueHeading()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/headingTrue", Value: trueHeading},
			)
		}
	}
	if v, ok := sentence.(MagneticHeading); ok {
		magneticHeading, _, err := v.GetMagneticHeading()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/headingMagnetic", Value: magneticHeading},
			)
		}
	}
	if v, ok := sentence.(NavigationStatus); ok {
		navigationStatus, _, err := v.GetNavigationStatus()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/state", Value: navigationStatus},
			)
		}
	}
	if v, ok := sentence.(NumberOfSatelites); ok {
		numberOfSatelites, _, err := v.GetNumberOfSatelites()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/gnss/satellites", Value: numberOfSatelites},
			)
		}
	}
	if v, ok := sentence.(Position2D); ok {
		lon, lat, _, err := v.GetPosition2D()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/position", Value: signalk.Position2D{Longitude: lon, Latitude: lat}},
			)
		}
	}
	if v, ok := sentence.(Position3D); ok {
		lon, lat, alt, _, err := v.GetPosition3D()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/position", Value: signalk.Position3D{Position2D: signalk.Position2D{Longitude: lon, Latitude: lat}, Altitude: alt}},
			)
		}
	}
	if v, ok := sentence.(SpeedOverGround); ok {
		speedOverGround, _, err := v.GetSpeedOverGround()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/speedOverGround", Value: speedOverGround},
			)
		}
	}
	if v, ok := sentence.(SpeedThroughWater); ok {
		speedThroughWater, _, err := v.GetSpeedThroughWater()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/speedThroughWater", Value: speedThroughWater},
			)
		}
	}
	if v, ok := sentence.(VesselName); ok {
		vesselName, _, err := v.GetVesselName()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "name", Value: vesselName},
			)
		}
	}
	if v, ok := sentence.(CallSign); ok {
		callSign, _, err := v.GetCallSign()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "communication/callsignVhf", Value: callSign},
			)
		}
	}
	if v, ok := sentence.(IMONumber); ok {
		imoNumber, _, err := v.GetIMONumber()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "registrations/imo", Value: imoNumber},
			)
		}
	}
	if v, ok := sentence.(ENINumber); ok {
		eniNumber, _, err := v.GetENINumber()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "registrations/other/eni/registration", Value: eniNumber},
			)
		}
	}
	if v, ok := sentence.(VesselDimensions); ok {
		length, beam, _, err := v.GetVesselDimensions()
		if err == nil {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "design/length", Value: signalk.OverallLength{Overall: length}},
			)
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "design/beam", Value: beam},
			)
		}
	}

	// if len(delta.Updates[0].Values) == 0 {
	// 	return delta, fmt.Errorf("Could not extract information from the sentence: %s", sentence)
	// }
	return delta, nil
}
