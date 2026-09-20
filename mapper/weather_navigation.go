package mapper

import (
	"math"
	"time"

	"github.com/munnik/gosk/message"
)

// timedFloat is a navigation value together with the timestamp of the
// message it came from. Everything the weather mapper reads from the
// navigation stream can go stale, a value is only usable while it is
// younger than the mapper's NavigationTimeout.
type timedFloat struct {
	value     float64
	timestamp time.Time
	valid     bool
}

func (t timedFloat) get(now time.Time, timeout time.Duration) (float64, bool) {
	if !t.valid || now.Sub(t.timestamp) > timeout {
		return 0, false
	}
	return t.value, true
}

func (t *timedFloat) set(value float64, timestamp time.Time) {
	t.value = value
	t.timestamp = timestamp
	t.valid = true
}

// navigationState is the latest navigation data the weather mapper has
// seen. It is only written from DoMap and only read from refreshMap, both
// of which run on process's single goroutine, so it needs no locking of
// its own.
type navigationState struct {
	latitude  float64
	longitude float64
	// positionTimestamp is the timestamp of the last position received,
	// zero while no position has been received at all.
	positionTimestamp time.Time
	// interval is the smoothed time between two consecutive position
	// updates, what the publish rate follows when navigation data arrives
	// slower than the configured minimum publish interval. Zero until at
	// least two positions have been received.
	interval time.Duration

	speedOverGround          timedFloat
	courseOverGroundTrue     timedFloat
	courseOverGroundMagnetic timedFloat
	headingTrue              timedFloat
	headingMagnetic          timedFloat
	magneticVariation        timedFloat
}

const (
	weatherPathPosition                 = "navigation.position"
	weatherPathSpeedOverGround          = "navigation.speedOverGround"
	weatherPathCourseOverGroundTrue     = "navigation.courseOverGroundTrue"
	weatherPathCourseOverGroundMagnetic = "navigation.courseOverGroundMagnetic"
	weatherPathHeadingTrue              = "navigation.headingTrue"
	weatherPathHeadingMagnetic          = "navigation.headingMagnetic"
	weatherPathMagneticVariation        = "navigation.magneticVariation"
)

// intervalSmoothing weighs the most recent gap between two positions
// against the interval measured so far. A plain "last gap" would make the
// publish rate jump on a single late message, the average of the two
// settles on a changed rate within a few messages without doing that.
const intervalSmoothing = 0.5

// update folds a single mapped value into the navigation state. Values on
// paths the weather mapper does not use are ignored.
func (n *navigationState) update(svm message.SingleValueMapped) {
	switch svm.Path {
	case weatherPathPosition:
		position, ok := svm.Value.(message.Position)
		if !ok || position.Latitude == nil || position.Longitude == nil {
			// an altitude only position, or a value that is not a
			// position at all, says nothing about where the vessel is
			return
		}
		n.latitude = *position.Latitude
		n.longitude = *position.Longitude
		n.updateInterval(svm.Timestamp)
	case weatherPathSpeedOverGround:
		n.setFloat(&n.speedOverGround, svm)
	case weatherPathCourseOverGroundTrue:
		n.setFloat(&n.courseOverGroundTrue, svm)
	case weatherPathCourseOverGroundMagnetic:
		n.setFloat(&n.courseOverGroundMagnetic, svm)
	case weatherPathHeadingTrue:
		n.setFloat(&n.headingTrue, svm)
	case weatherPathHeadingMagnetic:
		n.setFloat(&n.headingMagnetic, svm)
	case weatherPathMagneticVariation:
		n.setFloat(&n.magneticVariation, svm)
	}
}

func (n *navigationState) setFloat(target *timedFloat, svm message.SingleValueMapped) {
	if value, ok := asFloat(svm.Value); ok {
		target.set(value, svm.Timestamp)
	}
}

// updateInterval tracks how fast positions arrive. A gap is ignored when
// it is not positive (a repeated or out of order timestamp) so a burst of
// messages sharing one timestamp cannot drive the measured interval to
// zero.
func (n *navigationState) updateInterval(timestamp time.Time) {
	if !n.positionTimestamp.IsZero() {
		if gap := timestamp.Sub(n.positionTimestamp); gap > 0 {
			if n.interval == 0 {
				n.interval = gap
			} else {
				n.interval = time.Duration(intervalSmoothing*float64(gap) + (1-intervalSmoothing)*float64(n.interval))
			}
		}
	}
	n.positionTimestamp = timestamp
}

// position returns the vessel's position as long as it is still fresh.
func (n *navigationState) position(now time.Time, timeout time.Duration) (latitude float64, longitude float64, ok bool) {
	if n.positionTimestamp.IsZero() || now.Sub(n.positionTimestamp) > timeout {
		return 0, 0, false
	}
	return n.latitude, n.longitude, true
}

// heading returns the direction the bow points relative to true north,
// converting a magnetic heading when only that and the magnetic variation
// are available.
func (n *navigationState) heading(now time.Time, timeout time.Duration) (float64, bool) {
	if heading, ok := n.headingTrue.get(now, timeout); ok {
		return normalizeAngle(heading), true
	}
	return n.toTrue(n.headingMagnetic, now, timeout)
}

// courseOverGround returns the direction the vessel is travelling in
// relative to true north. It says nothing about where the vessel is
// pointing and is meaningless at (almost) zero speed over ground, callers
// check the speed before using it.
func (n *navigationState) courseOverGround(now time.Time, timeout time.Duration) (float64, bool) {
	if course, ok := n.courseOverGroundTrue.get(now, timeout); ok {
		return normalizeAngle(course), true
	}
	return n.toTrue(n.courseOverGroundMagnetic, now, timeout)
}

func (n *navigationState) toTrue(magnetic timedFloat, now time.Time, timeout time.Duration) (float64, bool) {
	value, ok := magnetic.get(now, timeout)
	if !ok {
		return 0, false
	}
	variation, ok := n.magneticVariation.get(now, timeout)
	if !ok {
		return 0, false
	}
	return normalizeAngle(value + variation), true
}

// windFrame is the vessel's movement as the apparent wind calculation
// needs it: a reference direction the vessel relative wind angles are
// measured from, and the velocity vector over ground that is subtracted
// from the true wind.
type windFrame struct {
	// reference is the direction, relative to true north, that
	// angleApparent and angleTrueGround are measured from.
	reference float64
	// course is the direction the vessel moves in over ground, only
	// meaningful when speed is not zero.
	course float64
	// speed is the speed over ground in m/s, zero when the vessel is not
	// moving (or is moving slower than the configured threshold).
	speed float64
}

// windFrame decides what the apparent wind can be calculated against, and
// reports false when it cannot be calculated at all.
//
// The heading is preferred over the course over ground as the reference:
// the apparent wind is what is felt on board, and what is felt is relative
// to where the vessel points, not to where it happens to be going. The
// course over ground is only usable as a stand in while the vessel is
// actually moving - at (almost) zero speed over ground it is derived from
// noise, a drifting vessel reports a course that swings through the full
// circle - so with no heading available and the vessel stopped there is
// nothing to measure an angle from and nothing is calculated.
//
// The velocity vector that is subtracted from the true wind is the course
// over ground at the speed over ground, since the true wind from the
// weather API is over ground as well. A moving vessel that reports no
// course over ground falls back on its heading, which ignores leeway and
// current but is far closer than pretending the vessel is stationary.
func (n *navigationState) windFrame(now time.Time, timeout time.Duration, minSpeed float64) (windFrame, bool) {
	speed := 0.0
	if sog, ok := n.speedOverGround.get(now, timeout); ok && sog >= minSpeed {
		speed = sog
	}
	heading, hasHeading := n.heading(now, timeout)
	course, hasCourse := n.courseOverGround(now, timeout)

	result := windFrame{speed: speed}
	switch {
	case hasHeading:
		result.reference = heading
	case speed > 0 && hasCourse:
		result.reference = course
	default:
		return windFrame{}, false
	}

	switch {
	case speed == 0:
		// the vessel is not moving, its velocity vector is zero and its
		// direction does not matter
		result.course = result.reference
	case hasCourse:
		result.course = course
	default:
		result.course = heading
	}

	return result, true
}

// isMoving reports whether the vessel is moving faster than minSpeed. A
// vessel whose speed over ground is unknown or stale counts as not moving,
// which only slows the publish rate down.
func (n *navigationState) isMoving(now time.Time, timeout time.Duration, minSpeed float64) bool {
	speed, ok := n.speedOverGround.get(now, timeout)
	return ok && speed >= minSpeed
}

// apparentWind returns the speed and the vessel relative angle of the wind
// as it is felt on board, given the true wind over ground and the vessel's
// movement.
//
// trueDirection is the direction the wind comes from relative to true
// north, trueSpeed its speed over ground. The returned angle is relative
// to frame.reference and is negative to port, as Signal K's
// environment.wind.angleApparent is defined.
//
// Both winds are handled as vectors in a north/east frame: the apparent
// wind is the true wind's velocity minus the vessel's own velocity, which
// is exactly what a wind instrument on a moving vessel measures. At zero
// speed over ground this reduces to the true wind, as it should.
func apparentWind(trueDirection float64, trueSpeed float64, frame windFrame) (speed float64, angle float64) {
	// the direction the air travels towards, the direction it comes from
	// turned around
	windNorth := trueSpeed * math.Cos(trueDirection+math.Pi)
	windEast := trueSpeed * math.Sin(trueDirection+math.Pi)

	vesselNorth := frame.speed * math.Cos(frame.course)
	vesselEast := frame.speed * math.Sin(frame.course)

	apparentNorth := windNorth - vesselNorth
	apparentEast := windEast - vesselEast

	speed = math.Hypot(apparentNorth, apparentEast)
	if speed == 0 {
		// no air movement relative to the vessel, there is no direction
		// to report and atan2 would return an arbitrary one
		return 0, 0
	}
	// turn the velocity back into the direction the apparent wind comes
	// from
	angle = normalizeAngle(math.Atan2(-apparentEast, -apparentNorth) - frame.reference)

	return speed, angle
}

// normalizeAngle folds an angle into (-pi, pi], the range Signal K uses
// for angles that are negative to port.
func normalizeAngle(angle float64) float64 {
	angle = math.Mod(angle, 2*math.Pi)
	if angle > math.Pi {
		angle -= 2 * math.Pi
	}
	if angle <= -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

// normalizeDirection folds an angle into [0, 2pi), the range Signal K
// uses for a compass direction. This is deliberately not the same range
// as normalizeAngle's: a direction is where something is relative to
// north and is never negative, an angle is relative to the vessel and is
// negative to port.
func normalizeDirection(angle float64) float64 {
	angle = math.Mod(angle, 2*math.Pi)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	return angle
}

// windChill is the wind chill temperature in K for a temperature in K and
// a wind speed in m/s, using the formula behind the North American wind
// chill index. It is only defined for temperatures at or below 10 degrees
// Celsius and wind speeds of at least 4.8 km/h, outside that range it
// returns false and nothing is published rather than a number the formula
// does not support.
func windChill(temperature float64, windSpeed float64) (float64, bool) {
	celsius := temperature - 273.15
	kilometersPerHour := windSpeed * 3.6
	if celsius > 10 || kilometersPerHour < 4.8 {
		return 0, false
	}
	factor := math.Pow(kilometersPerHour, 0.16)
	chill := 13.12 + 0.6215*celsius - 11.37*factor + 0.3965*celsius*factor

	return chill + 273.15, true
}

// heatIndex is the heat index temperature in K for a temperature in K and
// a relative humidity as a ratio, using the Rothfusz regression the US
// National Weather Service publishes, including its two adjustments for
// the dry and the humid corner of its range. Below 80 degrees Fahrenheit
// the regression is not valid and the heat index is not defined, which
// returns false.
func heatIndex(temperature float64, relativeHumidity float64) (float64, bool) {
	fahrenheit := (temperature-273.15)*9/5 + 32
	humidity := relativeHumidity * 100
	if fahrenheit < 80 || humidity < 0 || humidity > 100 {
		return 0, false
	}

	index := -42.379 +
		2.04901523*fahrenheit +
		10.14333127*humidity -
		0.22475541*fahrenheit*humidity -
		0.00683783*fahrenheit*fahrenheit -
		0.05481717*humidity*humidity +
		0.00122874*fahrenheit*fahrenheit*humidity +
		0.00085282*fahrenheit*humidity*humidity -
		0.00000199*fahrenheit*fahrenheit*humidity*humidity

	switch {
	case humidity < 13 && fahrenheit <= 112:
		index -= (13 - humidity) / 4 * math.Sqrt((17-math.Abs(fahrenheit-95))/17)
	case humidity > 85 && fahrenheit <= 87:
		index += (humidity - 85) / 10 * (87 - fahrenheit) / 5
	}

	return (index-32)*5/9 + 273.15, true
}

// asFloat converts a value from a mapped message to a float64. Values
// arrive as float64 or, when the producer sent a whole number, as int64,
// see message.Decode.
func asFloat(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int:
		return float64(typed), true
	}
	return 0, false
}
