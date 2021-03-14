package nmea

import (
	"fmt"

	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/signalk"
)

// KeyValueFromNMEA0183 tries to create a Signal K Delta from a NMEA sentence
func KeyValueFromNMEA0183(m *nanomsg.RawData) ([]signalk.Value, error) {
	result := make([]signalk.Value, 0)
	sentence, err := Parse(string(m.Payload))
	if err != nil {
		return result, err
	}

	context := ""
	if v, ok := sentence.(MMSI); ok {
		if mmsi, err := v.GetMMSI(); err == nil {
			context = fmt.Sprintf("vessels.urn:mrn:imo:mmsi:%d", mmsi)
		}
	}

	if v, ok := sentence.(DepthBelowSurface); ok {
		if depthBelowSurface, err := v.GetDepthBelowSurface(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "depth", "belowSurface"}, Value: depthBelowSurface})
		}
	}
	if v, ok := sentence.(DepthBelowTransducer); ok {
		if depthBelowTransducer, err := v.GetDepthBelowTransducer(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "depth", "belowTransducer"}, Value: depthBelowTransducer})
		}
	}
	if v, ok := sentence.(FixQuality); ok {
		if fixQuality, err := v.GetFixQuality(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "gnss", "methodQuality"}, Value: fixQuality})
		}
	}
	if v, ok := sentence.(FixType); ok {
		if fixType, err := v.GetFixType(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "gnss", "type"}, Value: fixType})
		}
	}
	if v, ok := sentence.(RateOfTurn); ok {
		if rateOfTurn, err := v.GetRateOfTurn(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "rateOfTurn"}, Value: rateOfTurn})
		}
	}
	if v, ok := sentence.(TrueCourseOverGround); ok {
		if trueCourseOverGround, err := v.GetTrueCourseOverGround(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "courseOverGroundTrue"}, Value: trueCourseOverGround})
		}
	}
	if v, ok := sentence.(TrueHeading); ok {
		if trueHeading, err := v.GetTrueHeading(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "headingTrue"}, Value: trueHeading})
		}
	}
	if v, ok := sentence.(MagneticHeading); ok {
		if magneticHeading, err := v.GetMagneticHeading(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "headingMagnetic"}, Value: magneticHeading})
		}
	}
	if v, ok := sentence.(NavigationStatus); ok {
		if navigationStatus, err := v.GetNavigationStatus(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "state"}, Value: navigationStatus})
		}
	}
	if v, ok := sentence.(NumberOfSatelites); ok {
		if numberOfSatelites, err := v.GetNumberOfSatelites(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "gnss", "satellites"}, Value: numberOfSatelites})
		}
	}
	if v, ok := sentence.(Position2D); ok {
		if lon, lat, err := v.GetPosition2D(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "position"}, Value: nanomsg.Position{Longitude: lon, Latitude: lat}})
		}
	}
	if v, ok := sentence.(Position3D); ok {
		if lon, lat, alt, err := v.GetPosition3D(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "position"}, Value: nanomsg.Position{Longitude: lon, Latitude: lat, Altitude: alt}})
		}
	}
	if v, ok := sentence.(SpeedOverGround); ok {
		if speedOverGround, err := v.GetSpeedOverGround(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "speedOverGround"}, Value: speedOverGround})
		}
	}
	if v, ok := sentence.(SpeedThroughWater); ok {
		if speedThroughWater, err := v.GetSpeedThroughWater(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "speedThroughWater"}, Value: speedThroughWater})
		}
	}
	if v, ok := sentence.(VesselName); ok {
		if vesselName, err := v.GetVesselName(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"name"}, Value: vesselName})
		}
	}
	if v, ok := sentence.(CallSign); ok {
		if callSign, err := v.GetCallSign(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"communication", "callsignVhf"}, Value: callSign})
		}
	}
	if v, ok := sentence.(IMONumber); ok {
		if imoNumber, err := v.GetIMONumber(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"registrations", "imo"}, Value: imoNumber})
		}
	}
	if v, ok := sentence.(ENINumber); ok {
		if eniNumber, err := v.GetENINumber(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"registrations", "other", "eni", "registration"}, Value: eniNumber})
		}
	}
	if v, ok := sentence.(VesselLength); ok {
		if length, err := v.GetVesselLength(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "length"}, Value: nanomsg.Length{Overall: length}})
		}
	}
	if v, ok := sentence.(VesselBeam); ok {
		if beam, err := v.GetVesselBeam(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "beam"}, Value: beam})
		}
	}
	if v, ok := sentence.(TrueWindDirection); ok {
		if trueWindDirection, err := v.GetTrueWindDirection(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "wind", "directionTrue"}, Value: trueWindDirection})
		}
	}
	if v, ok := sentence.(MagneticWindDirection); ok {
		if magneticWindDirection, err := v.GetMagneticWindDirection(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "wind", "directionMagnetic"}, Value: magneticWindDirection})
		}
	}
	if v, ok := sentence.(WindSpeed); ok {
		if windSpeed, err := v.GetWindSpeed(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "wind", "speedOverGround"}, Value: windSpeed})
		}
	}
	if v, ok := sentence.(OutsideTemperature); ok {
		if outsideTemperature, err := v.GetOutsideTemperature(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "outside", "temperature"}, Value: outsideTemperature})
		}
	}
	if v, ok := sentence.(DewPointTemperature); ok {
		if dewPointTemperature, err := v.GetDewPointTemperature(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "outside", "dewPointTemperature"}, Value: dewPointTemperature})
		}
	}
	if v, ok := sentence.(Humidity); ok {
		if humidity, err := v.GetHumidity(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "outside", "humidity"}, Value: humidity})
		}
	}
	if v, ok := sentence.(WaterTemperature); ok {
		if waterTemperature, err := v.GetWaterTemperature(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "water", "temperature"}, Value: waterTemperature})
		}
	}

	if len(result) == 0 {
		return result, fmt.Errorf("Data cannot be mapped: %s", sentence.String())
	}
	return result, nil
}
