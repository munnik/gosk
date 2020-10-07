package nmea

import (
	"fmt"
	"log"
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
		depthBelowSurface, err := v.GetDepthBelowSurface()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "environment/depth/belowSurface", Value: depthBelowSurface},
			)
		}
	}
	if v, ok := sentence.(DepthBelowTransducer); ok {
		depthBelowTransducer, err := v.GetDepthBelowTransducer()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "environment/depth/belowTransducer", Value: depthBelowTransducer},
			)
		}
	}
	if v, ok := sentence.(FixQuality); ok {
		fixQuality, err := v.GetFixQuality()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/gnss/methodQuality", Value: fixQuality},
			)
		}
	}
	if v, ok := sentence.(FixType); ok {
		fixType, err := v.GetFixType()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/gnss/type", Value: fixType},
			)
		}
	}
	if v, ok := sentence.(TrueCourseOverGround); ok {
		trueCourseOverGround, err := v.GetTrueCourseOverGround()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/courseOverGroundTrue", Value: trueCourseOverGround},
			)
		}
	}
	if v, ok := sentence.(TrueHeading); ok {
		trueHeading, err := v.GetTrueHeading()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/headingTrue", Value: trueHeading},
			)
		}
	}
	if v, ok := sentence.(MagneticHeading); ok {
		magneticHeading, err := v.GetMagneticHeading()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/headingMagnetic", Value: magneticHeading},
			)
		}
	}
	if v, ok := sentence.(NumberOfSatelites); ok {
		numberOfSatelites, err := v.GetNumberOfSatelites()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/gnss/satellites", Value: numberOfSatelites},
			)
		}
	}
	if v, ok := sentence.(Position2D); ok {
		lon, lat, err := v.GetPosition2D()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/position", Value: signalk.Position2D{Longitude: lon, Latitude: lat}},
			)
		}
	}
	if v, ok := sentence.(Position3D); ok {
		lon, lat, alt, err := v.GetPosition3D()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/position", Value: signalk.Position3D{Position2D: signalk.Position2D{Longitude: lon, Latitude: lat}, Altitude: alt}},
			)
		}
	}
	if v, ok := sentence.(SpeedOverGround); ok {
		speedOverGround, err := v.GetSpeedOverGround()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/speedOverGround", Value: speedOverGround},
			)
		}
	}
	if v, ok := sentence.(SpeedThroughWater); ok {
		speedThroughWater, err := v.GetSpeedThroughWater()
		if err != nil {
			log.Println(err)
		} else {
			delta.Updates[0].AppendValue(
				signalk.Value{Path: "navigation/speedThroughWater", Value: speedThroughWater},
			)
		}
	}

	if len(delta.Updates[0].Values) == 0 {
		return delta, fmt.Errorf("Could not extract information from the sentence: %s", sentence)
	}
	return delta, nil
}
