package mapper

import (
	"fmt"
	"net/url"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

// Signal K paths the weather mapper publishes. Where the Signal K
// specification has a path for a value it is used as is, see
// https://signalk.org/specification/1.7.0/doc/vesselsBranch.html. The
// specification has no paths at all for a sea state, and none for gusts
// or visibility either, so those are gosk's own - they are documented in
// SIGNALK_PATHS.md along with the rest of gosk's additions.
//
// Every value is in the Signal K base unit for its path: K for
// temperatures, Pa for pressure, ratio for humidity, m for heights and
// distances, s for periods, m/s for speeds and rad for angles.
const (
	weatherPathOutsideTemperature          = "environment.outside.temperature"
	weatherPathOutsideDewPoint             = "environment.outside.dewPointTemperature"
	weatherPathOutsideRelativeHumidity     = "environment.outside.relativeHumidity"
	weatherPathOutsidePressure             = "environment.outside.pressure"
	weatherPathOutsideHeatIndex            = "environment.outside.heatIndexTemperature"
	weatherPathOutsideApparentWindChill    = "environment.outside.apparentWindChillTemperature"
	weatherPathOutsideTheoreticalWindChill = "environment.outside.theoreticalWindChillTemperature"
	weatherPathOutsideVisibility           = "environment.outside.visibility"
	weatherPathWindSpeedOverGround         = "environment.wind.speedOverGround"
	weatherPathWindGust                    = "environment.wind.gust"
	weatherPathWindDirectionTrue           = "environment.wind.directionTrue"
	weatherPathWindDirectionMagnetic       = "environment.wind.directionMagnetic"
	weatherPathWindAngleTrueGround         = "environment.wind.angleTrueGround"
	weatherPathWindSpeedApparent           = "environment.wind.speedApparent"
	weatherPathWindAngleApparent           = "environment.wind.angleApparent"
	weatherPathWaterTemperature            = "environment.water.temperature"
	weatherPathCurrent                     = "environment.current"
	weatherPathWavesHeight                 = "environment.water.waves.significantHeight"
	weatherPathWavesDirection              = "environment.water.waves.direction"
	weatherPathWavesAngle                  = "environment.water.waves.angle"
	weatherPathWavesPeriod                 = "environment.water.waves.period"
	weatherPathWindWavesHeight             = "environment.water.waves.windWave.significantHeight"
	weatherPathWindWavesDirection          = "environment.water.waves.windWave.direction"
	weatherPathWindWavesPeriod             = "environment.water.waves.windWave.period"
	weatherPathSwellHeight                 = "environment.water.waves.swell.significantHeight"
	weatherPathSwellDirection              = "environment.water.waves.swell.direction"
	weatherPathSwellPeriod                 = "environment.water.waves.swell.period"
)

// WeatherMapper fills in the environment branch of a vessel from public
// weather APIs, using the vessel's own position to decide what weather to
// ask for.
//
// It subscribes to mapped data, not to raw data: its input is whatever
// mapper publishes the vessel's navigation branch (position, speed over
// ground, course over ground and heading), and its output is the
// environment branch for that same vessel. It deliberately does not pass
// its input through - the navigation data it reads is already published by
// the mapper it subscribes to, and echoing it here at the rate it arrives
// would both duplicate it downstream and defeat the whole point of
// publishing on a slow, fixed interval. Merge its output with the rest of
// the pipeline with a proxy, the way the other producers in a gosk
// pipeline are merged.
//
// Two endpoints are read, each through its own sourceFetcher so that
// neither can hold up or break the other: the forecast API for everything
// atmospheric, and the marine API for the sea state. The marine API has
// nothing to say about an inland waterway and answers with nulls there,
// which the fetcher turns into a long backoff for that grid cell, so a
// vessel that never leaves the Rhine effectively stops asking for waves
// altogether. See sourceFetcher.
//
// Publishing is driven by the ticker (see refreshMap and GetTickerInterval)
// rather than by incoming data, so the rate the environment branch is
// published at is decided by this mapper alone and never follows the rate
// a GPS happens to produce positions at.
type WeatherMapper struct {
	config config.WeatherMapperConfig
	air    *sourceFetcher[*weatherObservation]
	// marine is nil when no marine URL is configured, which turns the sea
	// state off entirely for a fleet that only ever sails inland.
	marine *sourceFetcher[*marineObservation]

	// navigation and lastPublish are only touched from DoMap and
	// refreshMap, which both run on process's single goroutine.
	navigation  navigationState
	lastPublish time.Time
}

func NewWeatherMapper(c config.WeatherMapperConfig) (*WeatherMapper, error) {
	if c.URL == "" {
		return nil, fmt.Errorf("no url configured for the weather API")
	}
	if _, err := url.Parse(c.URL); err != nil {
		return nil, fmt.Errorf("could not parse the url %v of the weather API: %w", c.URL, err)
	}
	air := newOpenMeteoSource(c.URL, c.Models, c.RequestTimeout)

	var marine source[*marineObservation]
	if c.MarineURL != "" {
		if _, err := url.Parse(c.MarineURL); err != nil {
			return nil, fmt.Errorf("could not parse the url %v of the marine API: %w", c.MarineURL, err)
		}
		marine = newOpenMeteoMarineSource(c.MarineURL, c.MarineModels, c.RequestTimeout)
	}

	return newWeatherMapper(c, air, marine), nil
}

// newWeatherMapper builds the mapper around its sources. It repeats the
// few bounds config.WeatherMapperConfig's own verify applies, so a
// configuration built in code rather than read from a configuration file
// can not end up with a publish rate below the minimum allowed one or with
// a cache that is unable to hold anything.
//
// marine may be nil, the sea state is then never fetched nor published.
func newWeatherMapper(c config.WeatherMapperConfig, air source[*weatherObservation], marine source[*marineObservation]) *WeatherMapper {
	defaults := config.DefaultWeatherMapperConfig()
	if c.MinPublishInterval < config.MinAllowedPublishInterval {
		c.MinPublishInterval = config.MinAllowedPublishInterval
	}
	if c.GridResolution <= 0 {
		c.GridResolution = defaults.GridResolution
	}
	if c.MaxDataAge <= 0 {
		c.MaxDataAge = defaults.MaxDataAge
	}
	if c.CacheSize < 1 {
		c.CacheSize = defaults.CacheSize
	}
	if c.MarineRefreshInterval <= 0 {
		c.MarineRefreshInterval = c.RefreshInterval
	}
	if c.InlandRetryInterval <= 0 {
		c.InlandRetryInterval = defaults.InlandRetryInterval
	}

	m := &WeatherMapper{
		config: c,
		air: newSourceFetcher(
			"weather",
			air,
			newWeatherCache[*weatherObservation](c.GridResolution, c.MaxDataAge, c.CacheSize),
			c.RefreshInterval,
			// an empty answer from the forecast API is reported as a
			// failed request, not cached, so this never applies to it
			c.RefreshInterval,
			c.MinRequestInterval,
			c.MaxRequestInterval,
			c.RequestTimeout,
		),
	}
	if marine != nil {
		m.marine = newSourceFetcher(
			"marine",
			marine,
			// the sea state is cached for longer than the maximum age of
			// an atmospheric observation: a vessel that stays inland has
			// to remember that its grid cell has no sea state for longer
			// than it would ever keep a temperature
			newWeatherCache[*marineObservation](c.GridResolution, maxDuration(c.MaxDataAge, c.InlandRetryInterval), c.CacheSize),
			c.MarineRefreshInterval,
			c.InlandRetryInterval,
			c.MinRequestInterval,
			c.MaxRequestInterval,
			c.RequestTimeout,
		)
	}

	return m
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// GetTickerInterval returns the interval on which the mapper is
// re-evaluated regardless of incoming data, see periodicMapper in main.go.
//
// This is not the publish interval, it is how often the decision to
// publish is taken (see publishInterval for the interval itself). Ticking
// at half the minimum publish interval keeps the actual publish rate close
// to the interval the configuration asks for: a tick exactly as long as
// the interval would round every publish up to the next tick, turning a 12
// second interval into a 20 second one.
func (m *WeatherMapper) GetTickerInterval() time.Duration {
	if m.config.Interval > 0 {
		return m.config.Interval
	}
	interval := m.config.MinPublishInterval / 2
	if interval < time.Second {
		interval = time.Second
	}
	return interval
}

func (m *WeatherMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	// empty updates are expected here, see DoMap, and are not worth a
	// warning for every single navigation message received
	process(subscriber, publisher, m, true)
}

// DoMap records the navigation data of the vessel this mapper is
// configured for and publishes nothing: the environment branch is built
// and published by refreshMap on the ticker instead, see the type's doc
// comment.
func (m *WeatherMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	if input.Context != m.config.Context && input.Context != "vessels.self" {
		// another vessel's navigation data says nothing about the weather
		// around this one
		return result, nil
	}

	for _, svm := range input.ToSingleValueMapped() {
		m.navigation.update(svm)
	}

	return result, nil
}

// refreshMap publishes the environment branch, at most once per
// publishInterval, and keeps the cached observations for the vessel's
// current position fresh.
//
// Nothing is published while the vessel's position is unknown or stale:
// weather that was fetched for a position the vessel may have left hours
// ago is worse than no weather at all. For the same reason an observation
// that aged past MaxDataAge is dropped from the cache instead of being
// published, so a vessel that loses its internet connection goes silent
// rather than reporting this morning's weather as if it were current.
func (m *WeatherMapper) refreshMap(now time.Time) *message.Mapped {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)

	latitude, longitude, ok := m.navigation.position(now, m.config.NavigationTimeout)
	if !ok {
		return result
	}

	m.air.request(now, latitude, longitude)
	if m.marine != nil {
		m.marine.request(now, latitude, longitude)
	}

	if now.Sub(m.lastPublish) < m.publishInterval(now) {
		return result
	}
	air, ok := m.air.observation(now, latitude, longitude)
	if !ok {
		return result
	}
	var marine *marineObservation
	if m.marine != nil {
		if observed, ok := m.marine.observation(now, latitude, longitude); ok && !observed.isEmpty() {
			marine = observed
		}
	}

	update := m.buildUpdate(air, marine, now)
	if len(update.Values) == 0 {
		return result
	}
	m.lastPublish = now

	return result.AddUpdate(update)
}

// publishInterval is the minimum time between two published environment
// updates as of now. It is never shorter than MinPublishInterval, never
// faster than the navigation data that feeds it, and slows down to
// StationaryPublishInterval while the vessel is not moving: a vessel that
// does not move stays in the same weather, and the apparent wind it feels
// stops changing with it.
func (m *WeatherMapper) publishInterval(now time.Time) time.Duration {
	interval := m.config.MinPublishInterval
	if m.navigation.interval > interval {
		interval = m.navigation.interval
	}
	if !m.navigation.isMoving(now, m.config.NavigationTimeout, m.config.MinSpeedOverGround) {
		if m.config.StationaryPublishInterval > interval {
			interval = m.config.StationaryPublishInterval
		}
	}

	return interval
}

// buildUpdate turns the observations, combined with what is currently
// known about the vessel's movement, into the environment branch. marine
// is nil when there is no sea state for this position, which is the
// normal case on an inland waterway.
//
// The update is timestamped with now and not with an observation's own
// timestamp: the apparent values in it are calculated from navigation data
// as it is now, and a weather model that publishes on the quarter hour
// would otherwise have every value in the pipeline look up to fifteen
// minutes old.
func (m *WeatherMapper) buildUpdate(observation *weatherObservation, marine *marineObservation, now time.Time) *message.Update {
	update := message.NewUpdate().
		WithSource(*message.NewSource().WithLabel("weather").WithType(config.WeatherType)).
		WithTimestamp(now)
	add := func(path string, value interface{}) {
		update.AddValue(message.NewValue().WithPath(path).WithValue(value))
	}
	addOptional := func(path string, value *float64) {
		if value != nil {
			add(path, *value)
		}
	}

	addOptional(weatherPathOutsideTemperature, observation.temperature)
	addOptional(weatherPathOutsideDewPoint, observation.dewPoint)
	addOptional(weatherPathOutsideRelativeHumidity, observation.relativeHumidity)
	addOptional(weatherPathOutsidePressure, observation.pressure)
	addOptional(weatherPathOutsideVisibility, observation.visibility)
	addOptional(weatherPathWindSpeedOverGround, observation.windSpeed)
	addOptional(weatherPathWindGust, observation.windGust)
	addOptional(weatherPathWindDirectionTrue, observation.windDirection)

	variation, hasVariation := m.navigation.magneticVariation.get(now, m.config.NavigationTimeout)
	if observation.windDirection != nil && hasVariation {
		add(weatherPathWindDirectionMagnetic, normalizeDirection(*observation.windDirection-variation))
	}
	if observation.temperature != nil && observation.relativeHumidity != nil {
		if index, ok := heatIndex(*observation.temperature, *observation.relativeHumidity); ok {
			add(weatherPathOutsideHeatIndex, index)
		}
	}
	if observation.temperature != nil && observation.windSpeed != nil {
		// the theoretical wind chill is the one the true wind causes, the
		// apparent one below additionally accounts for the vessel's own
		// movement through the air
		if chill, ok := windChill(*observation.temperature, *observation.windSpeed); ok {
			add(weatherPathOutsideTheoreticalWindChill, chill)
		}
	}

	// frame is what every vessel relative angle below is measured
	// against, and is not always available, see windFrame
	frame, hasFrame := m.navigation.windFrame(now, m.config.NavigationTimeout, m.config.MinSpeedOverGround)

	if observation.windDirection != nil && observation.windSpeed != nil && hasFrame {
		add(weatherPathWindAngleTrueGround, normalizeAngle(*observation.windDirection-frame.reference))
		speedApparent, angleApparent := apparentWind(*observation.windDirection, *observation.windSpeed, frame)
		add(weatherPathWindSpeedApparent, speedApparent)
		add(weatherPathWindAngleApparent, angleApparent)
		if observation.temperature != nil {
			if chill, ok := windChill(*observation.temperature, speedApparent); ok {
				add(weatherPathOutsideApparentWindChill, chill)
			}
		}
	}

	if marine == nil {
		return update
	}

	addOptional(weatherPathWaterTemperature, marine.seaSurfaceTemperature)
	addOptional(weatherPathWavesHeight, marine.waveHeight)
	addOptional(weatherPathWavesDirection, marine.waveDirection)
	addOptional(weatherPathWavesPeriod, marine.wavePeriod)
	addOptional(weatherPathWindWavesHeight, marine.windWaveHeight)
	addOptional(weatherPathWindWavesDirection, marine.windWaveDirection)
	addOptional(weatherPathWindWavesPeriod, marine.windWavePeriod)
	addOptional(weatherPathSwellHeight, marine.swellHeight)
	addOptional(weatherPathSwellDirection, marine.swellDirection)
	addOptional(weatherPathSwellPeriod, marine.swellPeriod)

	// the angle the sea runs at relative to the vessel, which is what
	// decides whether it rolls, pitches or slams, measured from the same
	// reference as the wind angles
	if marine.waveDirection != nil && hasFrame {
		add(weatherPathWavesAngle, normalizeAngle(*marine.waveDirection-frame.reference))
	}

	// environment.current is one of the few object valued paths in the
	// Signal K specification, see message.Current
	if marine.currentSpeed != nil || marine.currentDirection != nil {
		current := message.Current{Drift: marine.currentSpeed, SetTrue: marine.currentDirection}
		if marine.currentDirection != nil && hasVariation {
			setMagnetic := normalizeDirection(*marine.currentDirection - variation)
			current.SetMagnetic = &setMagnetic
		}
		add(weatherPathCurrent, current)
	}

	return update
}
