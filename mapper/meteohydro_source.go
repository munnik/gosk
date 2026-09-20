package mapper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// observation is what every source returns: a set of values for one
// position, any of which the provider may have had nothing to say about.
type observation interface {
	// isEmpty reports whether the provider returned nothing usable at
	// all. For the forecast API that is a failed request, for the marine
	// API it is the normal answer over an inland waterway, see
	// openMeteoMarineSource.
	isEmpty() bool
}

// weatherObservation is the provider independent set of atmospheric
// values the meteo/hydro mapper knows how to publish. Everything is already
// in SignalK units, a nil field is a value this provider did not report
// for this position.
type weatherObservation struct {
	// timestamp is the moment the provider says the observation is valid
	// for, not the moment it was fetched.
	timestamp        time.Time
	temperature      *float64 // K
	dewPoint         *float64 // K
	relativeHumidity *float64 // ratio, 0..1
	pressure         *float64 // Pa
	visibility       *float64 // m
	// windSpeed is the true wind speed over ground, m/s.
	windSpeed *float64
	// windGust is the maximum gust over the last hour, m/s.
	windGust *float64
	// windDirection is the direction the wind is coming from relative to
	// true north, rad.
	windDirection *float64
}

func (o *weatherObservation) isEmpty() bool {
	return o.temperature == nil &&
		o.dewPoint == nil &&
		o.relativeHumidity == nil &&
		o.pressure == nil &&
		o.visibility == nil &&
		o.windSpeed == nil &&
		o.windGust == nil &&
		o.windDirection == nil
}

// marineObservation is the sea state for one position. Every direction is
// the direction the waves are coming from relative to true north, the
// same convention the wind direction uses.
type marineObservation struct {
	timestamp time.Time
	// waveHeight is the significant height of the combined wind waves and
	// swell, m. The wind wave and swell fields below are that same sea
	// split into the part driven by the local wind and the part that
	// travelled in from elsewhere.
	waveHeight    *float64 // m
	waveDirection *float64 // rad
	wavePeriod    *float64 // s

	windWaveHeight    *float64 // m
	windWaveDirection *float64 // rad
	windWavePeriod    *float64 // s

	swellHeight    *float64 // m
	swellDirection *float64 // rad
	swellPeriod    *float64 // s

	seaSurfaceTemperature *float64 // K
	// currentSpeed is the drift of the ocean current, m/s, and
	// currentDirection the direction it sets towards relative to true
	// north, rad.
	currentSpeed     *float64
	currentDirection *float64
}

func (o *marineObservation) isEmpty() bool {
	return o.waveHeight == nil &&
		o.waveDirection == nil &&
		o.wavePeriod == nil &&
		o.windWaveHeight == nil &&
		o.swellHeight == nil &&
		o.seaSurfaceTemperature == nil &&
		o.currentSpeed == nil
}

// source fetches the current values for a position. It is an interface so
// the mapper can be tested without an HTTP server, and so a second
// provider can be added later without touching the mapper.
type source[T observation] interface {
	// fetch returns the current observation for latitude/longitude, both
	// in degrees.
	fetch(ctx context.Context, latitude float64, longitude float64) (T, error)
}

// maxWeatherResponseSize caps how much of a response body is read, a
// misconfigured URL pointing at something that is not a weather API should
// not be able to exhaust this process's memory.
const maxWeatherResponseSize = 1 << 20

// openMeteoCurrentVariables are the "current" variables requested from the
// forecast API, in the order the fields of openMeteoResponse expect them.
// Only variables that map onto a SignalK path are requested, asking for
// more would only make the responses bigger.
var openMeteoCurrentVariables = []string{
	"temperature_2m",
	"relative_humidity_2m",
	"dew_point_2m",
	"surface_pressure",
	"pressure_msl",
	"visibility",
	"wind_speed_10m",
	"wind_direction_10m",
	"wind_gusts_10m",
}

// openMeteoMarineVariables are the "current" variables requested from the
// marine API.
var openMeteoMarineVariables = []string{
	"wave_height",
	"wave_direction",
	"wave_period",
	"wind_wave_height",
	"wind_wave_direction",
	"wind_wave_period",
	"swell_wave_height",
	"swell_wave_direction",
	"swell_wave_period",
	"sea_surface_temperature",
	"ocean_current_velocity",
	"ocean_current_direction",
}

// openMeteoSource reads the current weather from an Open-Meteo compatible
// forecast API, see https://open-meteo.com/en/docs. The public API needs
// no API key and no account.
//
// models, when set, is passed through as the API's models parameter. Left
// empty the API picks the highest resolution model that covers the
// position by itself, which across north western Europe means KNMI
// Harmonie, Meteo-France AROME or DWD ICON-D2 depending on where the
// vessel is - exactly what a fleet spread over several countries wants.
type openMeteoSource struct {
	url    string
	models string
	client *http.Client
}

func newOpenMeteoSource(rawURL string, models string, timeout time.Duration) *openMeteoSource {
	return &openMeteoSource{url: rawURL, models: models, client: &http.Client{Timeout: timeout}}
}

type openMeteoResponse struct {
	// Error and Reason are what the API returns instead of data for an
	// invalid request, e.g. an out of range latitude.
	Error  bool   `json:"error"`
	Reason string `json:"reason"`

	Current struct {
		// Time is a unix timestamp, the mapper asks for
		// timeformat=unixtime so this does not need a timezone aware
		// parse of a local, zoneless timestamp.
		Time             int64    `json:"time"`
		Temperature      *float64 `json:"temperature_2m"`
		RelativeHumidity *float64 `json:"relative_humidity_2m"`
		DewPoint         *float64 `json:"dew_point_2m"`
		SurfacePressure  *float64 `json:"surface_pressure"`
		PressureMsl      *float64 `json:"pressure_msl"`
		Visibility       *float64 `json:"visibility"`
		WindSpeed        *float64 `json:"wind_speed_10m"`
		WindDirection    *float64 `json:"wind_direction_10m"`
		WindGusts        *float64 `json:"wind_gusts_10m"`
	} `json:"current"`
}

func (s *openMeteoSource) fetch(ctx context.Context, latitude float64, longitude float64) (*weatherObservation, error) {
	query := url.Values{}
	query.Set("current", strings.Join(openMeteoCurrentVariables, ","))
	query.Set("wind_speed_unit", "ms")
	if s.models != "" {
		query.Set("models", s.models)
	}

	body, err := getWeather(ctx, s.client, s.url, latitude, longitude, query)
	if err != nil {
		return nil, err
	}

	var parsed openMeteoResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("could not parse the response of the weather API: %w", err)
	}
	if parsed.Error {
		return nil, fmt.Errorf("the weather API returned an error: %v", truncate(parsed.Reason, 200))
	}

	result := &weatherObservation{timestamp: observationTime(parsed.Current.Time)}
	result.temperature = celsiusToKelvin(parsed.Current.Temperature)
	result.dewPoint = celsiusToKelvin(parsed.Current.DewPoint)
	result.relativeHumidity = percentToRatio(parsed.Current.RelativeHumidity)
	// surface pressure is the pressure at the vessel, which is what
	// environment.outside.pressure is, fall back on the sea level
	// pressure when the model did not report it
	if pressure := hectoPascalToPascal(parsed.Current.SurfacePressure); pressure != nil {
		result.pressure = pressure
	} else {
		result.pressure = hectoPascalToPascal(parsed.Current.PressureMsl)
	}
	result.visibility = parsed.Current.Visibility // already in m
	result.windSpeed = parsed.Current.WindSpeed   // requested in m/s
	result.windGust = parsed.Current.WindGusts    // requested in m/s
	result.windDirection = degreesToRadians(parsed.Current.WindDirection)

	if result.isEmpty() {
		return nil, fmt.Errorf("the weather API did not return any usable value for latitude %v and longitude %v", latitude, longitude)
	}

	return result, nil
}

// openMeteoMarineSource reads the sea state from an Open-Meteo compatible
// marine API, see https://open-meteo.com/en/docs/marine-weather-api. This
// is a different endpoint from the forecast API, with the same request and
// response shape.
//
// A position that is not at sea is not an error: the API answers with a
// null for every variable, which becomes an observation that isEmpty. That
// is the normal case for a vessel on an inland waterway, and the fetcher
// uses it to stop asking for that grid cell for a long while rather than
// spending a request per refresh interval on a canal that will never have
// a sea state.
type openMeteoMarineSource struct {
	url    string
	models string
	client *http.Client
}

func newOpenMeteoMarineSource(rawURL string, models string, timeout time.Duration) *openMeteoMarineSource {
	return &openMeteoMarineSource{url: rawURL, models: models, client: &http.Client{Timeout: timeout}}
}

type openMeteoMarineResponse struct {
	Error  bool   `json:"error"`
	Reason string `json:"reason"`

	Current struct {
		Time                  int64    `json:"time"`
		WaveHeight            *float64 `json:"wave_height"`
		WaveDirection         *float64 `json:"wave_direction"`
		WavePeriod            *float64 `json:"wave_period"`
		WindWaveHeight        *float64 `json:"wind_wave_height"`
		WindWaveDirection     *float64 `json:"wind_wave_direction"`
		WindWavePeriod        *float64 `json:"wind_wave_period"`
		SwellWaveHeight       *float64 `json:"swell_wave_height"`
		SwellWaveDirection    *float64 `json:"swell_wave_direction"`
		SwellWavePeriod       *float64 `json:"swell_wave_period"`
		SeaSurfaceTemperature *float64 `json:"sea_surface_temperature"`
		CurrentVelocity       *float64 `json:"ocean_current_velocity"`
		CurrentDirection      *float64 `json:"ocean_current_direction"`
	} `json:"current"`
}

func (s *openMeteoMarineSource) fetch(ctx context.Context, latitude float64, longitude float64) (*marineObservation, error) {
	query := url.Values{}
	query.Set("current", strings.Join(openMeteoMarineVariables, ","))
	// the marine API ignores a velocity unit parameter and always answers
	// in km/h, see kilometersPerHourToMetersPerSecond below
	query.Set("length_unit", "metric")
	if s.models != "" {
		query.Set("models", s.models)
	}

	body, err := getWeather(ctx, s.client, s.url, latitude, longitude, query)
	if err != nil {
		return nil, err
	}

	var parsed openMeteoMarineResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("could not parse the response of the marine API: %w", err)
	}
	if parsed.Error {
		return nil, fmt.Errorf("the marine API returned an error: %v", truncate(parsed.Reason, 200))
	}

	return &marineObservation{
		timestamp:             observationTime(parsed.Current.Time),
		waveHeight:            parsed.Current.WaveHeight,
		waveDirection:         degreesToRadians(parsed.Current.WaveDirection),
		wavePeriod:            parsed.Current.WavePeriod,
		windWaveHeight:        parsed.Current.WindWaveHeight,
		windWaveDirection:     degreesToRadians(parsed.Current.WindWaveDirection),
		windWavePeriod:        parsed.Current.WindWavePeriod,
		swellHeight:           parsed.Current.SwellWaveHeight,
		swellDirection:        degreesToRadians(parsed.Current.SwellWaveDirection),
		swellPeriod:           parsed.Current.SwellWavePeriod,
		seaSurfaceTemperature: celsiusToKelvin(parsed.Current.SeaSurfaceTemperature),
		currentSpeed:          kilometersPerHourToMetersPerSecond(parsed.Current.CurrentVelocity),
		currentDirection:      degreesToRadians(parsed.Current.CurrentDirection),
	}, nil
}

// getWeather performs the request both Open-Meteo endpoints share:
// latitude, longitude and the timestamp format are always the same, the
// caller supplies the rest of the query.
func getWeather(ctx context.Context, client *http.Client, rawURL string, latitude float64, longitude float64, query url.Values) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("could not parse the url %v: %w", rawURL, err)
	}
	// 4 decimals is about 10m, far beyond what any weather model
	// resolves, and keeps the URL (and with it any upstream cache key)
	// stable while the vessel moves within a grid cell.
	query.Set("latitude", strconv.FormatFloat(latitude, 'f', 4, 64))
	query.Set("longitude", strconv.FormatFloat(longitude, 'f', 4, 64))
	query.Set("timeformat", "unixtime")
	query.Set("timezone", "GMT")
	u.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxWeatherResponseSize))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		// the API puts the reason an otherwise valid looking request was
		// rejected in the body, include it, truncated, instead of only
		// reporting the status code
		return nil, fmt.Errorf("the weather API returned %v: %v", response.Status, truncate(string(body), 200))
	}

	return body, nil
}

// observationTime turns the unix timestamp of a response into a time,
// falling back on the moment of the request rather than on the unix epoch
// when the response carried no usable observation time.
func observationTime(unix int64) time.Time {
	if unix == 0 {
		return time.Now().UTC()
	}
	return time.Unix(unix, 0).UTC()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func celsiusToKelvin(celsius *float64) *float64 {
	if celsius == nil {
		return nil
	}
	result := *celsius + 273.15
	return &result
}

func hectoPascalToPascal(hectoPascal *float64) *float64 {
	if hectoPascal == nil {
		return nil
	}
	result := *hectoPascal * 100
	return &result
}

func percentToRatio(percent *float64) *float64 {
	if percent == nil {
		return nil
	}
	result := *percent / 100
	return &result
}

func degreesToRadians(degrees *float64) *float64 {
	if degrees == nil {
		return nil
	}
	result := *degrees * math.Pi / 180
	return &result
}

func kilometersPerHourToMetersPerSecond(kilometersPerHour *float64) *float64 {
	if kilometersPerHour == nil {
		return nil
	}
	result := *kilometersPerHour / 3.6
	return &result
}

// observationCacheEntry is a single fetched observation, kept until it ages
// past the mapper's MaxDataAge or is evicted to keep the cache bounded.
type observationCacheEntry[T observation] struct {
	observation T
	// fetched is when this entry was retrieved, the age the refresh and
	// max age checks are against. The observation's own timestamp is not
	// used for that: a model that publishes on the quarter hour reports a
	// timestamp up to 15 minutes old the moment it is fetched, which would
	// have every entry look stale immediately.
	fetched time.Time
}

// observationCache holds one observation per grid cell so a vessel that keeps
// reporting positions within the same cell is served from memory instead
// of from the weather API. It is safe for concurrent use: entries are
// written by the goroutine doing the fetch and read by the mapper's own
// goroutine.
type observationCache[T observation] struct {
	mutex      sync.Mutex
	resolution float64
	maxAge     time.Duration
	maxEntries int
	entries    map[string]*observationCacheEntry[T]
}

func newObservationCache[T observation](resolution float64, maxAge time.Duration, maxEntries int) *observationCache[T] {
	return &observationCache[T]{
		resolution: resolution,
		maxAge:     maxAge,
		maxEntries: maxEntries,
		entries:    make(map[string]*observationCacheEntry[T]),
	}
}

// key is the identity of the grid cell a position falls in. Weather is
// fetched, and reused, per cell: with the default resolution of 0.1 deg
// that is a cell of roughly 11 km of latitude, well inside the resolution
// of the weather models behind the public APIs.
func (c *observationCache[T]) key(latitude float64, longitude float64) string {
	return fmt.Sprintf(
		"%d/%d",
		int64(math.Round(latitude/c.resolution)),
		int64(math.Round(longitude/c.resolution)),
	)
}

// get returns the entry for the cell containing this position, if there is
// one that has not aged past maxAge.
func (c *observationCache[T]) get(latitude float64, longitude float64, now time.Time) (*observationCacheEntry[T], bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry, ok := c.entries[c.key(latitude, longitude)]
	if !ok || now.Sub(entry.fetched) > c.maxAge {
		return nil, false
	}
	return entry, true
}

func (c *observationCache[T]) put(latitude float64, longitude float64, now time.Time, observation T) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.entries[c.key(latitude, longitude)] = &observationCacheEntry[T]{observation: observation, fetched: now}
	c.evict(now)
}

// evict drops everything that aged past maxAge and, if that still leaves
// more entries than maxEntries, the oldest ones until it fits. The cache
// is small (a cell per default is 11 km wide, a vessel at 20 knots enters
// a new one every 18 minutes) so a linear scan is cheaper than maintaining
// a separate ordering. The caller holds the mutex.
func (c *observationCache[T]) evict(now time.Time) {
	for key, entry := range c.entries {
		if now.Sub(entry.fetched) > c.maxAge {
			delete(c.entries, key)
		}
	}
	for len(c.entries) > c.maxEntries {
		var oldestKey string
		var oldest time.Time
		for key, entry := range c.entries {
			if oldest.IsZero() || entry.fetched.Before(oldest) {
				oldestKey, oldest = key, entry.fetched
			}
		}
		delete(c.entries, oldestKey)
	}
}
