package mapper

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/munnik/go-nmea"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type Nmea0183Mapper struct {
	config   config.MapperConfig
	protocol string
}

func NewNmea0183Mapper(c config.MapperConfig) (*Nmea0183Mapper, error) {
	return &Nmea0183Mapper{config: c, protocol: config.NMEA0183Type}, nil
}

func (m *Nmea0183Mapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m)
}

func (m *Nmea0183Mapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	options := []string{}
	if optionsString, ok := m.config.ProtocolOptions[config.ProtocolOptionNmeaParse]; ok {
		options = strings.FieldsFunc(
			optionsString,
			func(c rune) bool {
				return !unicode.IsLetter(c) && !unicode.IsNumber(c)
			},
		)
	}

	sentence, err := nmea.Parse(string(r.Value), options...)
	if err != nil {
		return nil, err
	}

	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)

	// if it is a multi fragment message return without error if it is not the last fragment
	if aisSentence, ok := sentence.(nmea.VDMVDO); ok {
		if numFragments, err := aisSentence.NumFragments.GetValue(); err == nil {
			if fragmentNUmber, err := aisSentence.FragmentNumber.GetValue(); err == nil {
				if numFragments > fragmentNUmber {
					return result, nil
				}
			}
		}
	}

	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	if v, ok := sentence.(nmea.MMSI); ok {
		if mmsi, err := v.GetMMSI(); err == nil {
			result.WithContext(fmt.Sprintf("vessels.urn:mrn:imo:mmsi:%s", mmsi))
			u.AddValue(message.NewValue().WithPath("").WithValue(message.VesselInfo{MMSI: &mmsi}))
		}
	}
	if v, ok := sentence.(nmea.VesselName); ok {
		if vesselName, err := v.GetVesselName(); err == nil {
			u.AddValue(message.NewValue().WithPath("").WithValue(message.VesselInfo{Name: &vesselName}))
		}
	}
	if v, ok := sentence.(nmea.VesselType); ok {
		if vesselType, err := v.GetVesselType(); err == nil {
			u.AddValue(message.NewValue().WithPath("design.aisShipType").WithValue(message.VesselType{Name: &vesselType}))
		}
	}

	if v, ok := sentence.(nmea.DepthBelowSurface); ok {
		if depthBelowSurface, err := v.GetDepthBelowSurface(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.depth.belowSurface").WithValue(depthBelowSurface))
		}
	}
	if v, ok := sentence.(nmea.DepthBelowTransducer); ok {
		if depthBelowTransducer, err := v.GetDepthBelowTransducer(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.depth.belowTransducer").WithValue(depthBelowTransducer))
		}
	}
	if v, ok := sentence.(nmea.FixQuality); ok {
		if fixQuality, err := v.GetFixQuality(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.gnss.methodQuality").WithValue(fixQuality))
		}
	}
	if v, ok := sentence.(nmea.FixType); ok {
		if fixType, err := v.GetFixType(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.gnss.type").WithValue(fixType))
		}
	}
	if v, ok := sentence.(nmea.RateOfTurn); ok {
		if rateOfTurn, err := v.GetRateOfTurn(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.rateOfTurn").WithValue(rateOfTurn))
		}
	}
	if v, ok := sentence.(nmea.TrueCourseOverGround); ok {
		if trueCourseOverGround, err := v.GetTrueCourseOverGround(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.courseOverGroundTrue").WithValue(trueCourseOverGround))
		}
	}
	if v, ok := sentence.(nmea.TrueHeading); ok {
		if trueHeading, err := v.GetTrueHeading(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.headingTrue").WithValue(trueHeading))
		}
	}
	if v, ok := sentence.(nmea.MagneticHeading); ok {
		if magneticHeading, err := v.GetMagneticHeading(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.headingMagnetic").WithValue(magneticHeading))
		}
	}
	if v, ok := sentence.(nmea.NavigationStatus); ok {
		if navigationStatus, err := v.GetNavigationStatus(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.state").WithValue(navigationStatus))
		}
	}
	if v, ok := sentence.(nmea.NumberOfSatellites); ok {
		if numberOfSatellites, err := v.GetNumberOfSatellites(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.gnss.satellites").WithValue(numberOfSatellites))
		}
	}
	if v, ok := sentence.(nmea.Position2D); ok {
		if lat, lon, err := v.GetPosition2D(); err == nil {
			// TODO: omitempty in JSON for missing altitude
			u.AddValue(message.NewValue().WithPath("navigation.position").WithValue(message.Position{Latitude: &lat, Longitude: &lon}))
		}
	}
	if v, ok := sentence.(nmea.Position3D); ok {
		if lat, lon, alt, err := v.GetPosition3D(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.position").WithValue(message.Position{Altitude: &alt, Latitude: &lat, Longitude: &lon}))
		}
	}
	if v, ok := sentence.(nmea.SpeedOverGround); ok {
		if speedOverGround, err := v.GetSpeedOverGround(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.speedOverGround").WithValue(speedOverGround))
		}
	}
	if v, ok := sentence.(nmea.SpeedThroughWater); ok {
		if speedThroughWater, err := v.GetSpeedThroughWater(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.speedThroughWater").WithValue(speedThroughWater))
		}
	}
	if v, ok := sentence.(nmea.CallSign); ok {
		if callSign, err := v.GetCallSign(); err == nil {
			u.AddValue(message.NewValue().WithPath("communication.callsignVhf").WithValue(callSign))
		}
	}
	if v, ok := sentence.(nmea.IMONumber); ok {
		if imoNumber, err := v.GetIMONumber(); err == nil {
			u.AddValue(message.NewValue().WithPath("registrations.imo").WithValue(imoNumber))
		}
	}
	if v, ok := sentence.(nmea.ENINumber); ok {
		if eniNumber, err := v.GetENINumber(); err == nil {
			u.AddValue(message.NewValue().WithPath("registrations.other.eni.registration").WithValue(eniNumber))
		}
	}
	if v, ok := sentence.(nmea.VesselLength); ok {
		if length, err := v.GetVesselLength(); err == nil {
			// TODO: omitempty in JSON for missing hull and waterline
			u.AddValue(message.NewValue().WithPath("design.length").WithValue(message.Length{Overall: &length}))
		}
	}
	if v, ok := sentence.(nmea.VesselBeam); ok {
		if beam, err := v.GetVesselBeam(); err == nil {
			u.AddValue(message.NewValue().WithPath("design.beam").WithValue(beam))
		}
	}
	if v, ok := sentence.(nmea.RelativeWindDirection); ok {
		if relativeWindDirection, err := v.GetRelativeWindDirection(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.wind.angleApparent").WithValue(relativeWindDirection))
		}
	}
	if v, ok := sentence.(nmea.TrueWindDirection); ok {
		if trueWindDirection, err := v.GetTrueWindDirection(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.wind.directionTrue").WithValue(trueWindDirection))
		}
	}
	if v, ok := sentence.(nmea.MagneticWindDirection); ok {
		if magneticWindDirection, err := v.GetMagneticWindDirection(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.wind.directionMagnetic").WithValue(magneticWindDirection))
		}
	}
	if v, ok := sentence.(nmea.WindSpeed); ok {
		if windSpeed, err := v.GetWindSpeed(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.wind.speedOverGround").WithValue(windSpeed))
		}
	}
	if v, ok := sentence.(nmea.OutsideTemperature); ok {
		if outsideTemperature, err := v.GetOutsideTemperature(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.outside.temperature").WithValue(outsideTemperature))
		}
	}
	if v, ok := sentence.(nmea.DewPointTemperature); ok {
		if dewPointTemperature, err := v.GetDewPointTemperature(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.outside.dewPointTemperature").WithValue(dewPointTemperature))
		}
	}
	if v, ok := sentence.(nmea.Humidity); ok {
		if humidity, err := v.GetHumidity(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.outside.humidity").WithValue(humidity))
		}
	}
	if v, ok := sentence.(nmea.WaterTemperature); ok {
		if waterTemperature, err := v.GetWaterTemperature(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.water.temperature").WithValue(waterTemperature))
		}
	}
	if v, ok := sentence.(nmea.Heave); ok {
		if heave, err := v.GetHeave(); err == nil {
			u.AddValue(message.NewValue().WithPath("environment.heave").WithValue(heave))
		}
	}
	if v, ok := sentence.(nmea.DateTime); ok {
		if dt, err := v.GetDateTime(); err == nil {
			u.AddValue(message.NewValue().WithPath("navigation.datetime").WithValue(dt))
		}
	}
	if v, ok := sentence.(nmea.RudderAngle); ok {
		rudderAngleStarboard, errStarboard := v.GetRudderAngleStarboard()
		rudderAnglePortSide, errPort := v.GetRudderAnglePortside()
		if rudderAngle, err := v.GetRudderAngle(); err == nil {
			u.AddValue(message.NewValue().WithPath("steering.rudderAngle").WithValue(rudderAngle))
		} else if errStarboard == nil && errPort == nil {
			u.AddValue(message.NewValue().WithPath("steering.rudderAngle").WithValue((rudderAngleStarboard + rudderAnglePortSide) / 2))
		} else if errStarboard == nil {
			u.AddValue(message.NewValue().WithPath("steering.rudderAngle").WithValue(rudderAngleStarboard))
		} else if errPort == nil {
			u.AddValue(message.NewValue().WithPath("steering.rudderAngle").WithValue(rudderAnglePortSide))
		}
	}
	if v, ok := sentence.(nmea.Alarm); ok {
		description, _ := v.GetDescription()
		active, err := v.IsActive()
		if err != nil {
			active = true
		}
		u.AddValue(message.NewValue().WithPath("notifications.ais").WithValue(message.Alarm{State: &active, Message: &description}))
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %s", sentence.String())
	}

	return result.AddUpdate(u), nil
}
