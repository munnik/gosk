package mapper

import (
	"fmt"
	"net/url"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

// Signal K paths the meteo/hydro mapper publishes. Where the Signal K
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
	meteoHydroPathOutsideTemperature          = "environment.outside.temperature"
	meteoHydroPathOutsideDewPoint             = "environment.outside.dewPointTemperature"
	meteoHydroPathOutsideRelativeHumidity     = "environment.outside.relativeHumidity"
	meteoHydroPathOutsidePressure             = "environment.outside.pressure"
	meteoHydroPathOutsideHeatIndex            = "environment.outside.heatIndexTemperature"
	meteoHydroPathOutsideApparentWindChill    = "environment.outside.apparentWindChillTemperature"
	meteoHydroPathOutsideTheoreticalWindChill = "environment.outside.theoreticalWindChillTemperature"
	meteoHydroPathOutsideVisibility           = "environment.outside.visibility"
	meteoHydroPathWindSpeedOverGround         = "environment.wind.speedOverGround"
	meteoHydroPathWindGust                    = "environment.wind.gust"
	meteoHydroPathWindDirectionTrue           = "environment.wind.directionTrue"
	meteoHydroPathWindDirectionMagnetic       = "environment.wind.directionMagnetic"
	meteoHydroPathWindAngleTrueGround         = "environment.wind.angleTrueGround"
	meteoHydroPathWindSpeedApparent           = "environment.wind.speedApparent"
	meteoHydroPathWindAngleApparent           = "environment.wind.angleApparent"
	meteoHydroPathWaterTemperature            = "environment.water.temperature"
	meteoHydroPathCurrent                     = "environment.current"
	meteoHydroPathWavesHeight                 = "environment.water.waves.significantHeight"
	meteoHydroPathWavesDirection              = "environment.water.waves.direction"
	meteoHydroPathWavesAngle                  = "environment.water.waves.angle"
	meteoHydroPathWavesPeriod                 = "environment.water.waves.period"
	meteoHydroPathWindWavesHeight             = "environment.water.waves.windWave.significantHeight"
	meteoHydroPathWindWavesDirection          = "environment.water.waves.windWave.direction"
	meteoHydroPathWindWavesPeriod             = "environment.water.waves.windWave.period"
	meteoHydroPathSwellHeight                 = "environment.water.waves.swell.significantHeight"
	meteoHydroPathSwellDirection              = "environment.water.waves.swell.direction"
	meteoHydroPathSwellPeriod                 = "environment.water.waves.swell.period"
)

// meteoHydroPaths is every path this mapper can publish, and so every
// path on which a real instrument's data has to take precedence over the
// model, see hasLiveData.
var meteoHydroPaths = []string{
	meteoHydroPathOutsideTemperature,
	meteoHydroPathOutsideDewPoint,
	meteoHydroPathOutsideRelativeHumidity,
	meteoHydroPathOutsidePressure,
	meteoHydroPathOutsideHeatIndex,
	meteoHydroPathOutsideApparentWindChill,
	meteoHydroPathOutsideTheoreticalWindChill,
	meteoHydroPathOutsideVisibility,
	meteoHydroPathWindSpeedOverGround,
	meteoHydroPathWindGust,
	meteoHydroPathWindDirectionTrue,
	meteoHydroPathWindDirectionMagnetic,
	meteoHydroPathWindAngleTrueGround,
	meteoHydroPathWindSpeedApparent,
	meteoHydroPathWindAngleApparent,
	meteoHydroPathWaterTemperature,
	meteoHydroPathCurrent,
	meteoHydroPathWavesHeight,
	meteoHydroPathWavesDirection,
	meteoHydroPathWavesAngle,
	meteoHydroPathWavesPeriod,
	meteoHydroPathWindWavesHeight,
	meteoHydroPathWindWavesDirection,
	meteoHydroPathWindWavesPeriod,
	meteoHydroPathSwellHeight,
	meteoHydroPathSwellDirection,
	meteoHydroPathSwellPeriod,
}

// MeteoHydroMapper fills in the environment branch of a vessel from public
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
//
// A modelled value is never as good as an instrument on the vessel
// itself, so any path another source is currently publishing is left
// alone entirely, per path and for as long as that source keeps
// reporting - see hasLiveData. That is what lets this mapper run on a
// vessel that does have a wind sensor: it fills in the temperature, the
// pressure and the sea state, stays out of the way of the anemometer,
// and takes the wind over by itself if the anemometer ever goes quiet.
// It therefore wants to see every other mapper on the vessel, not only
// the navigation ones - see subscribeTo in the nix configuration.
type MeteoHydroMapper struct {
	config config.MeteoHydroMapperConfig
	air    *sourceFetcher[*weatherObservation]
	// marine is nil when no marine URL is configured, which turns the sea
	// state off entirely for a fleet that only ever sails inland.
	marine *sourceFetcher[*marineObservation]

	// navigation, liveData and lastPublish are only touched from DoMap
	// and refreshMap, which both run on process's single goroutine.
	navigation navigationState
	// liveData records, per path this mapper can publish, when another
	// source last published a value for it.
	liveData    map[string]time.Time
	lastPublish time.Time
}

func NewMeteoHydroMapper(c config.MeteoHydroMapperConfig) (*MeteoHydroMapper, error) {
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

	return newMeteoHydroMapper(c, air, marine), nil
}

// newMeteoHydroMapper builds the mapper around its sources. It repeats the
// few bounds config.MeteoHydroMapperConfig's own verify applies, so a
// configuration built in code rather than read from a configuration file
// can not end up with a publish rate below the minimum allowed one or with
// a cache that is unable to hold anything.
//
// marine may be nil, the sea state is then never fetched nor published.
func newMeteoHydroMapper(c config.MeteoHydroMapperConfig, air source[*weatherObservation], marine source[*marineObservation]) *MeteoHydroMapper {
	defaults := config.DefaultMeteoHydroMapperConfig()
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
	if c.LiveDataTimeout <= 0 {
		c.LiveDataTimeout = defaults.LiveDataTimeout
	}

	m := &MeteoHydroMapper{
		config:   c,
		liveData: make(map[string]time.Time, len(meteoHydroPaths)),
		air: newSourceFetcher(
			"weather",
			air,
			newObservationCache[*weatherObservation](c.GridResolution, c.MaxDataAge, c.CacheSize),
			c.RefreshInterval,
			// an empty answer from the forecast API is reported as a
			// failed request, not cached, so this never applies to it
			c.RefreshInterval,
			c.MinRequestInterval,
			c.MaxRequestInterval,
			c.RequestTimeout,
		),
	}
	for _, path := range meteoHydroPaths {
		m.liveData[path] = time.Time{}
	}
	if marine != nil {
		m.marine = newSourceFetcher(
			"marine",
			marine,
			// the sea state is cached for longer than the maximum age of
			// an atmospheric observation: a vessel that stays inland has
			// to remember that its grid cell has no sea state for longer
			// than it would ever keep a temperature
			newObservationCache[*marineObservation](c.GridResolution, maxDuration(c.MaxDataAge, c.InlandRetryInterval), c.CacheSize),
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
func (m *MeteoHydroMapper) GetTickerInterval() time.Duration {
	if m.config.Interval > 0 {
		return m.config.Interval
	}
	interval := m.config.MinPublishInterval / 2
	if interval < time.Second {
		interval = time.Second
	}
	return interval
}

func (m *MeteoHydroMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	// empty updates are expected here, see DoMap, and are not worth a
	// warning for every single navigation message received
	process(subscriber, publisher, m, true)
}

// DoMap records the navigation data of the vessel this mapper is
// configured for and publishes nothing: the environment branch is built
// and published by refreshMap on the ticker instead, see the type's doc
// comment.
func (m *MeteoHydroMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	if input.Context != m.config.Context && input.Context != "vessels.self" {
		// another vessel's navigation data says nothing about the weather
		// around this one
		return result, nil
	}

	for _, svm := range input.ToSingleValueMapped() {
		if svm.Source.Type == config.MeteoHydroType {
			// this mapper's own output, coming back because it
			// subscribes to every mapper on the vessel (itself
			// excluded in the configuration, but a second instance,
			// or a value that travelled around some other way, would
			// otherwise have it forever defer to itself)
			continue
		}
		m.navigation.update(svm)
		if _, ok := m.liveData[svm.Path]; ok {
			m.liveData[svm.Path] = svm.Timestamp
		}
	}

	return result, nil
}

// hasLiveData reports whether another source published this path recently
// enough to still be trusted over the model. A source that goes quiet for
// LiveDataTimeout - an instrument that fails, or a connector that loses
// its serial port - hands the path back to this mapper rather than
// leaving it empty.
func (m *MeteoHydroMapper) hasLiveData(path string, now time.Time) bool {
	last, seen := m.liveData[path]
	return seen && !last.IsZero() && now.Sub(last) <= m.config.LiveDataTimeout
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
func (m *MeteoHydroMapper) refreshMap(now time.Time) *message.Mapped {
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
func (m *MeteoHydroMapper) publishInterval(now time.Time) time.Duration {
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
func (m *MeteoHydroMapper) buildUpdate(observation *weatherObservation, marine *marineObservation, now time.Time) *message.Update {
	update := message.NewUpdate().
		WithSource(*message.NewSource().WithLabel("meteohydro").WithType(config.MeteoHydroType)).
		WithTimestamp(now)
	add := func(path string, value interface{}) {
		if m.hasLiveData(path, now) {
			// an instrument on board is reporting this path, its
			// value is the real one - see the type's doc comment
			return
		}
		update.AddValue(message.NewValue().WithPath(path).WithValue(value))
	}
	addOptional := func(path string, value *float64) {
		if value != nil {
			add(path, *value)
		}
	}

	addOptional(meteoHydroPathOutsideTemperature, observation.temperature)
	addOptional(meteoHydroPathOutsideDewPoint, observation.dewPoint)
	addOptional(meteoHydroPathOutsideRelativeHumidity, observation.relativeHumidity)
	addOptional(meteoHydroPathOutsidePressure, observation.pressure)
	addOptional(meteoHydroPathOutsideVisibility, observation.visibility)
	addOptional(meteoHydroPathWindSpeedOverGround, observation.windSpeed)
	addOptional(meteoHydroPathWindGust, observation.windGust)
	addOptional(meteoHydroPathWindDirectionTrue, observation.windDirection)

	variation, hasVariation := m.navigation.magneticVariation.get(now, m.config.NavigationTimeout)
	if observation.windDirection != nil && hasVariation {
		add(meteoHydroPathWindDirectionMagnetic, normalizeDirection(*observation.windDirection-variation))
	}
	if observation.temperature != nil && observation.relativeHumidity != nil {
		if index, ok := heatIndex(*observation.temperature, *observation.relativeHumidity); ok {
			add(meteoHydroPathOutsideHeatIndex, index)
		}
	}
	if observation.temperature != nil && observation.windSpeed != nil {
		// the theoretical wind chill is the one the true wind causes, the
		// apparent one below additionally accounts for the vessel's own
		// movement through the air
		if chill, ok := windChill(*observation.temperature, *observation.windSpeed); ok {
			add(meteoHydroPathOutsideTheoreticalWindChill, chill)
		}
	}

	// frame is what every vessel relative angle below is measured
	// against, and is not always available, see windFrame
	frame, hasFrame := m.navigation.windFrame(now, m.config.NavigationTimeout, m.config.MinSpeedOverGround)

	if observation.windDirection != nil && observation.windSpeed != nil && hasFrame {
		add(meteoHydroPathWindAngleTrueGround, normalizeAngle(*observation.windDirection-frame.reference))
		speedApparent, angleApparent := apparentWind(*observation.windDirection, *observation.windSpeed, frame)
		add(meteoHydroPathWindSpeedApparent, speedApparent)
		add(meteoHydroPathWindAngleApparent, angleApparent)
		if observation.temperature != nil {
			if chill, ok := windChill(*observation.temperature, speedApparent); ok {
				add(meteoHydroPathOutsideApparentWindChill, chill)
			}
		}
	}

	if marine == nil {
		return update
	}

	addOptional(meteoHydroPathWaterTemperature, marine.seaSurfaceTemperature)
	addOptional(meteoHydroPathWavesHeight, marine.waveHeight)
	addOptional(meteoHydroPathWavesDirection, marine.waveDirection)
	addOptional(meteoHydroPathWavesPeriod, marine.wavePeriod)
	addOptional(meteoHydroPathWindWavesHeight, marine.windWaveHeight)
	addOptional(meteoHydroPathWindWavesDirection, marine.windWaveDirection)
	addOptional(meteoHydroPathWindWavesPeriod, marine.windWavePeriod)
	addOptional(meteoHydroPathSwellHeight, marine.swellHeight)
	addOptional(meteoHydroPathSwellDirection, marine.swellDirection)
	addOptional(meteoHydroPathSwellPeriod, marine.swellPeriod)

	// the angle the sea runs at relative to the vessel, which is what
	// decides whether it rolls, pitches or slams, measured from the same
	// reference as the wind angles
	if marine.waveDirection != nil && hasFrame {
		add(meteoHydroPathWavesAngle, normalizeAngle(*marine.waveDirection-frame.reference))
	}

	// environment.current is one of the few object valued paths in the
	// Signal K specification, see message.Current
	if marine.currentSpeed != nil || marine.currentDirection != nil {
		current := message.Current{Drift: marine.currentSpeed, SetTrue: marine.currentDirection}
		if marine.currentDirection != nil && hasVariation {
			setMagnetic := normalizeDirection(*marine.currentDirection - variation)
			current.SetMagnetic = &setMagnetic
		}
		add(meteoHydroPathCurrent, current)
	}

	return update
}
