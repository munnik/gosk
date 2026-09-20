package mapper

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

// Signal K paths the weather mapper publishes, see
// https://signalk.org/specification/1.7.0/doc/vesselsBranch.html. Every
// value is in the Signal K base unit for its path: K for temperatures, Pa
// for pressure, ratio for humidity, m/s for speeds and rad for angles.
const (
	weatherPathOutsideTemperature          = "environment.outside.temperature"
	weatherPathOutsideDewPoint             = "environment.outside.dewPointTemperature"
	weatherPathOutsideRelativeHumidity     = "environment.outside.relativeHumidity"
	weatherPathOutsidePressure             = "environment.outside.pressure"
	weatherPathOutsideHeatIndex            = "environment.outside.heatIndexTemperature"
	weatherPathOutsideApparentWindChill    = "environment.outside.apparentWindChillTemperature"
	weatherPathOutsideTheoreticalWindChill = "environment.outside.theoreticalWindChillTemperature"
	weatherPathWindSpeedOverGround         = "environment.wind.speedOverGround"
	weatherPathWindDirectionTrue           = "environment.wind.directionTrue"
	weatherPathWindDirectionMagnetic       = "environment.wind.directionMagnetic"
	weatherPathWindAngleTrueGround         = "environment.wind.angleTrueGround"
	weatherPathWindSpeedApparent           = "environment.wind.speedApparent"
	weatherPathWindAngleApparent           = "environment.wind.angleApparent"
)

// WeatherMapper fills in the environment branch of a vessel from a public
// weather API, using the vessel's own position to decide what weather to
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
// Publishing is driven by the ticker (see refreshMap and GetTickerInterval)
// rather than by incoming data, so the rate the environment branch is
// published at is decided by this mapper alone and never follows the rate
// a GPS happens to produce positions at. The weather API is polled far
// less often than that, see requestWeather: observations are cached per
// grid cell and reused, so a vessel sitting in a harbour makes one request
// per RefreshInterval no matter how fast it publishes, and a moving vessel
// only adds a request when it leaves the cell its last observation was
// fetched for.
type WeatherMapper struct {
	config config.WeatherMapperConfig
	source weatherSource
	cache  *weatherCache

	// navigation and lastPublish are only touched from DoMap and
	// refreshMap, which both run on process's single goroutine.
	navigation  navigationState
	lastPublish time.Time

	// fetchMutex guards the fields below, which are shared with the
	// goroutine that performs a request.
	fetchMutex sync.Mutex
	fetching   bool
	// nextRequest is the earliest moment a new request may be started, it
	// enforces MinRequestInterval between successful requests and the
	// exponential backoff after a failed one.
	nextRequest    time.Time
	requestBackoff *backoff.Backoff
}

func NewWeatherMapper(c config.WeatherMapperConfig) (*WeatherMapper, error) {
	if c.URL == "" {
		return nil, fmt.Errorf("no url configured for the weather API")
	}
	if _, err := url.Parse(c.URL); err != nil {
		return nil, fmt.Errorf("could not parse the url %v of the weather API: %w", c.URL, err)
	}

	return newWeatherMapper(c, newOpenMeteoSource(c.URL, c.RequestTimeout)), nil
}

// newWeatherMapper builds the mapper around a weather source. It repeats
// the few bounds config.WeatherMapperConfig's own verify applies, so a
// configuration built in code rather than read from a configuration file
// can not end up with a publish rate below the minimum allowed one or with
// a cache that is unable to hold anything.
func newWeatherMapper(c config.WeatherMapperConfig, source weatherSource) *WeatherMapper {
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

	return &WeatherMapper{
		config: c,
		source: source,
		cache:  newWeatherCache(c.GridResolution, c.MaxDataAge, c.CacheSize),
		requestBackoff: &backoff.Backoff{
			Min:    c.MinRequestInterval,
			Max:    c.MaxRequestInterval,
			Factor: 2,
			Jitter: true,
		},
	}
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
// publishInterval, and keeps the cached observation for the vessel's
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

	m.requestWeather(now, latitude, longitude)

	if now.Sub(m.lastPublish) < m.publishInterval(now) {
		return result
	}
	entry, ok := m.cache.get(latitude, longitude, now)
	if !ok {
		return result
	}
	update := m.buildUpdate(entry.observation, now)
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

// requestWeather starts a request for this position when the cache cannot
// answer it, at most one at a time and never sooner than the backoff and
// MinRequestInterval allow. It returns immediately, the request itself
// runs on its own goroutine: refreshMap runs on the same goroutine as the
// rest of the pipeline's processing, blocking it on a network request
// would stall every message behind it.
func (m *WeatherMapper) requestWeather(now time.Time, latitude float64, longitude float64) {
	if entry, ok := m.cache.get(latitude, longitude, now); ok && now.Sub(entry.fetched) < m.config.RefreshInterval {
		// the observation for the grid cell the vessel is in is still
		// fresh, this is what keeps a vessel that publishes every 10
		// seconds from making a request every 10 seconds
		return
	}

	m.fetchMutex.Lock()
	if m.fetching || now.Before(m.nextRequest) {
		m.fetchMutex.Unlock()
		return
	}
	m.fetching = true
	m.fetchMutex.Unlock()

	go m.fetch(now, latitude, longitude)
}

// fetch performs a single request and caches its result. now is the moment
// the request was started, which is what the entry's age is measured from.
func (m *WeatherMapper) fetch(now time.Time, latitude float64, longitude float64) {
	ctx, cancel := context.WithTimeout(context.Background(), m.config.RequestTimeout)
	defer cancel()

	observation, err := m.source.fetch(ctx, latitude, longitude)
	if err == nil {
		m.cache.put(latitude, longitude, now, observation)
	}

	m.fetchMutex.Lock()
	defer m.fetchMutex.Unlock()
	m.fetching = false
	if err != nil {
		// back off exponentially, an API that is down, rate limiting us
		// or rejecting our requests should not be asked again every
		// MinRequestInterval for as long as the vessel is under way
		wait := m.requestBackoff.Duration()
		m.nextRequest = now.Add(wait)
		logger.GetLogger().Warn(
			"Could not get the weather for the current position",
			zap.Float64("Latitude", latitude),
			zap.Float64("Longitude", longitude),
			zap.Duration("Retrying in", wait),
			zap.String("Error", err.Error()),
		)
		return
	}
	m.requestBackoff.Reset()
	m.nextRequest = now.Add(m.config.MinRequestInterval)
}

// buildUpdate turns an observation, combined with what is currently known
// about the vessel's movement, into the environment branch.
//
// The update is timestamped with now and not with the observation's own
// timestamp: the apparent values in it are calculated from navigation data
// as it is now, and a weather model that publishes on the quarter hour
// would otherwise have every value in the pipeline look up to fifteen
// minutes old.
func (m *WeatherMapper) buildUpdate(observation *weatherObservation, now time.Time) *message.Update {
	update := message.NewUpdate().
		WithSource(*message.NewSource().WithLabel("weather").WithType(config.WeatherType)).
		WithTimestamp(now)
	add := func(path string, value float64) {
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
	addOptional(weatherPathWindSpeedOverGround, observation.windSpeed)
	addOptional(weatherPathWindDirectionTrue, observation.windDirection)

	if observation.windDirection != nil {
		if variation, ok := m.navigation.magneticVariation.get(now, m.config.NavigationTimeout); ok {
			add(weatherPathWindDirectionMagnetic, normalizeAngle(*observation.windDirection-variation))
		}
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

	if observation.windDirection == nil || observation.windSpeed == nil {
		return update
	}
	frame, ok := m.navigation.windFrame(now, m.config.NavigationTimeout, m.config.MinSpeedOverGround)
	if !ok {
		// nothing to measure a vessel relative angle from, see windFrame:
		// no heading, and a course over ground that cannot be trusted
		// because the vessel is (almost) stopped
		return update
	}

	add(weatherPathWindAngleTrueGround, normalizeAngle(*observation.windDirection-frame.reference))
	speedApparent, angleApparent := apparentWind(*observation.windDirection, *observation.windSpeed, frame)
	add(weatherPathWindSpeedApparent, speedApparent)
	add(weatherPathWindAngleApparent, angleApparent)
	if observation.temperature != nil {
		if chill, ok := windChill(*observation.temperature, speedApparent); ok {
			add(weatherPathOutsideApparentWindChill, chill)
		}
	}

	return update
}
