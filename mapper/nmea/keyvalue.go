package nmea

import (
	"fmt"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/mapper/signalk"
	"github.com/munnik/gosk/nanomsg"
)

// KeyValueFromNMEA0183 tries to create a Signal K Delta from a NMEA sentence
func KeyValueFromNMEA0183(m *nanomsg.RawData, cfg config.NMEA0183Config) ([]signalk.Value, error) {
	result := make([]signalk.Value, 0)
	sentence, err := Parse(string(m.Payload))
	if err != nil {
		return result, err
	}

	context := cfg.Context
	if v, ok := sentence.(MMSI); ok {
		if mmsi, err := v.GetMMSI(); err == nil {
			context = fmt.Sprintf("vessels.urn:mrn:imo:mmsi:%d", mmsi)
			result = append(result, signalk.Value{Context: context, Path: []string{""}, Value: &nanomsg.VesselDataValue{Mmsi: &mmsi}})
		}
	}

	if v, ok := sentence.(DepthBelowSurface); ok {
		if depthBelowSurface, err := v.GetDepthBelowSurface(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "depth", "belowSurface"}, Value: nanomsg.Double(depthBelowSurface)})
		}
	}
	if v, ok := sentence.(DepthBelowTransducer); ok {
		if depthBelowTransducer, err := v.GetDepthBelowTransducer(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "depth", "belowTransducer"}, Value: nanomsg.Double(depthBelowTransducer)})
		}
	}
	if v, ok := sentence.(FixQuality); ok {
		if fixQuality, err := v.GetFixQuality(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "gnss", "methodQuality"}, Value: nanomsg.String(fixQuality)})
		}
	}
	if v, ok := sentence.(FixType); ok {
		if fixType, err := v.GetFixType(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "gnss", "type"}, Value: nanomsg.String(fixType)})
		}
	}
	if v, ok := sentence.(RateOfTurn); ok {
		if rateOfTurn, err := v.GetRateOfTurn(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "rateOfTurn"}, Value: nanomsg.Double(rateOfTurn)})
		}
	}
	if v, ok := sentence.(TrueCourseOverGround); ok {
		if trueCourseOverGround, err := v.GetTrueCourseOverGround(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "courseOverGroundTrue"}, Value: nanomsg.Double(trueCourseOverGround)})
		}
	}
	if v, ok := sentence.(TrueHeading); ok {
		if trueHeading, err := v.GetTrueHeading(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "headingTrue"}, Value: nanomsg.Double(trueHeading)})
		}
	}
	if v, ok := sentence.(MagneticHeading); ok {
		if magneticHeading, err := v.GetMagneticHeading(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "headingMagnetic"}, Value: nanomsg.Double(magneticHeading)})
		}
	}
	if v, ok := sentence.(NavigationStatus); ok {
		if navigationStatus, err := v.GetNavigationStatus(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "state"}, Value: nanomsg.String(navigationStatus)})
		}
	}
	if v, ok := sentence.(NumberOfSatelites); ok {
		if numberOfSatelites, err := v.GetNumberOfSatelites(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "gnss", "satellites"}, Value: nanomsg.Int64(numberOfSatelites)})
		}
	}
	if v, ok := sentence.(Position2D); ok {
		if lat, lon, err := v.GetPosition2D(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "position"}, Value: &nanomsg.PositionValue{Longitude: &lon, Latitude: &lat}})
		}
	}
	if v, ok := sentence.(Position3D); ok {
		if lat, lon, alt, err := v.GetPosition3D(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "position"}, Value: &nanomsg.PositionValue{Longitude: &lon, Latitude: &lat, Altitude: &alt}})
		}
	}
	if v, ok := sentence.(SpeedOverGround); ok {
		if speedOverGround, err := v.GetSpeedOverGround(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "speedOverGround"}, Value: nanomsg.Double(speedOverGround)})
		}
	}
	if v, ok := sentence.(SpeedThroughWater); ok {
		if speedThroughWater, err := v.GetSpeedThroughWater(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"navigation", "speedThroughWater"}, Value: nanomsg.Double(speedThroughWater)})
		}
	}
	if v, ok := sentence.(VesselName); ok {
		if vesselName, err := v.GetVesselName(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{""}, Value: &nanomsg.VesselDataValue{Name: &vesselName}})
		}
	}
	if v, ok := sentence.(CallSign); ok {
		if callSign, err := v.GetCallSign(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"communication", "callsignVhf"}, Value: nanomsg.String(callSign)})
		}
	}
	if v, ok := sentence.(IMONumber); ok {
		if imoNumber, err := v.GetIMONumber(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"registrations", "imo"}, Value: nanomsg.String(imoNumber)})
		}
	}
	if v, ok := sentence.(ENINumber); ok {
		if eniNumber, err := v.GetENINumber(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"registrations", "other", "eni", "registration"}, Value: nanomsg.String(eniNumber)})
		}
	}
	if v, ok := sentence.(VesselLength); ok {
		if length, err := v.GetVesselLength(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "length"}, Value: &nanomsg.LengthValue{Overall: &length}})
		}
	}
	if v, ok := sentence.(VesselBeam); ok {
		if beam, err := v.GetVesselBeam(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"design", "beam"}, Value: nanomsg.Double(beam)})
		}
	}
	if v, ok := sentence.(TrueWindDirection); ok {
		if trueWindDirection, err := v.GetTrueWindDirection(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "wind", "directionTrue"}, Value: nanomsg.Double(trueWindDirection)})
		}
	}
	if v, ok := sentence.(MagneticWindDirection); ok {
		if magneticWindDirection, err := v.GetMagneticWindDirection(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "wind", "directionMagnetic"}, Value: nanomsg.Double(magneticWindDirection)})
		}
	}
	if v, ok := sentence.(WindSpeed); ok {
		if windSpeed, err := v.GetWindSpeed(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "wind", "speedOverGround"}, Value: nanomsg.Double(windSpeed)})
		}
	}
	if v, ok := sentence.(OutsideTemperature); ok {
		if outsideTemperature, err := v.GetOutsideTemperature(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "outside", "temperature"}, Value: nanomsg.Double(outsideTemperature)})
		}
	}
	if v, ok := sentence.(DewPointTemperature); ok {
		if dewPointTemperature, err := v.GetDewPointTemperature(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "outside", "dewPointTemperature"}, Value: nanomsg.Double(dewPointTemperature)})
		}
	}
	if v, ok := sentence.(Humidity); ok {
		if humidity, err := v.GetHumidity(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "outside", "humidity"}, Value: nanomsg.Double(humidity)})
		}
	}
	if v, ok := sentence.(WaterTemperature); ok {
		if waterTemperature, err := v.GetWaterTemperature(); err == nil {
			result = append(result, signalk.Value{Context: context, Path: []string{"environment", "water", "temperature"}, Value: nanomsg.Double(waterTemperature)})
		}
	}

	if len(result) == 0 {
		return result, fmt.Errorf("data cannot be mapped: %s", sentence.String())
	}
	return result, nil
}
