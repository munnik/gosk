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

// weatherObservation is the provider independent set of values the weather
// mapper knows how to publish. Everything is already in SignalK units, a
// nil field is a value this provider did not report for this position.
type weatherObservation struct {
	// timestamp is the moment the provider says the observation is valid
	// for, not the moment it was fetched.
	timestamp        time.Time
	temperature      *float64 // K
	dewPoint         *float64 // K
	relativeHumidity *float64 // ratio, 0..1
	pressure         *float64 // Pa
	// windSpeed is the true wind speed over ground, m/s.
	windSpeed *float64
	// windDirection is the direction the wind is coming from relative to
	// true north, rad.
	windDirection *float64
}

// isEmpty reports whether the provider returned nothing usable at all,
// which is treated as a failed request rather than as an observation with
// no values in it.
func (o *weatherObservation) isEmpty() bool {
	return o.temperature == nil &&
		o.dewPoint == nil &&
		o.relativeHumidity == nil &&
		o.pressure == nil &&
		o.windSpeed == nil &&
		o.windDirection == nil
}

// weatherSource fetches the current weather for a position. It is an
// interface so the mapper can be tested without an HTTP server, and so a
// second provider can be added later without touching the mapper.
type weatherSource interface {
	// fetch returns the current observation for latitude/longitude, both
	// in degrees.
	fetch(ctx context.Context, latitude, longitude float64) (*weatherObservation, error)
}

// maxWeatherResponseSize caps how much of a response body is read, a
// misconfigured URL pointing at something that is not a weather API should
// not be able to exhaust this process's memory.
const maxWeatherResponseSize = 1 << 20

// openMeteoCurrentVariables are the "current" variables requested from the
// API, in the order the fields of openMeteoResponse expect them. Only
// variables that map onto a SignalK path are requested, asking for more
// would only make the responses bigger.
var openMeteoCurrentVariables = []string{
	"temperature_2m",
	"relative_humidity_2m",
	"dew_point_2m",
	"surface_pressure",
	"pressure_msl",
	"wind_speed_10m",
	"wind_direction_10m",
}

// openMeteoSource reads the current weather from an Open-Meteo compatible
// forecast API, see https://open-meteo.com/en/docs. The public API needs
// no API key and no account.
type openMeteoSource struct {
	url    string
	client *http.Client
}

func newOpenMeteoSource(rawURL string, timeout time.Duration) *openMeteoSource {
	return &openMeteoSource{url: rawURL, client: &http.Client{Timeout: timeout}}
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
		WindSpeed        *float64 `json:"wind_speed_10m"`
		WindDirection    *float64 `json:"wind_direction_10m"`
	} `json:"current"`
}

func (s *openMeteoSource) fetch(ctx context.Context, latitude, longitude float64) (*weatherObservation, error) {
	u, err := url.Parse(s.url)
	if err != nil {
		return nil, fmt.Errorf("could not parse the weather API url %v: %w", s.url, err)
	}
	q := u.Query()
	// 4 decimals is about 10m, far beyond what any weather model
	// resolves, and keeps the URL (and with it any upstream cache key)
	// stable while the vessel moves within a grid cell.
	q.Set("latitude", strconv.FormatFloat(latitude, 'f', 4, 64))
	q.Set("longitude", strconv.FormatFloat(longitude, 'f', 4, 64))
	q.Set("current", strings.Join(openMeteoCurrentVariables, ","))
	q.Set("wind_speed_unit", "ms")
	q.Set("timeformat", "unixtime")
	q.Set("timezone", "GMT")
	u.RawQuery = q.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Do(request)
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

	var parsed openMeteoResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("could not parse the response of the weather API: %w", err)
	}
	if parsed.Error {
		return nil, fmt.Errorf("the weather API returned an error: %v", truncate(parsed.Reason, 200))
	}

	result := &weatherObservation{timestamp: time.Unix(parsed.Current.Time, 0).UTC()}
	if parsed.Current.Time == 0 {
		// no usable observation time, fall back on the moment of the
		// request rather than on the unix epoch
		result.timestamp = time.Now().UTC()
	}
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
	result.windSpeed = parsed.Current.WindSpeed // requested in m/s
	result.windDirection = degreesToRadians(parsed.Current.WindDirection)

	if result.isEmpty() {
		return nil, fmt.Errorf("the weather API did not return any usable value for latitude %v and longitude %v", latitude, longitude)
	}

	return result, nil
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

// weatherCacheEntry is a single fetched observation, kept until it ages
// past the mapper's MaxDataAge or is evicted to keep the cache bounded.
type weatherCacheEntry struct {
	observation *weatherObservation
	// fetched is when this entry was retrieved, the age the refresh and
	// max age checks are against. The observation's own timestamp is not
	// used for that: a model that publishes on the quarter hour reports a
	// timestamp up to 15 minutes old the moment it is fetched, which would
	// have every entry look stale immediately.
	fetched time.Time
}

// weatherCache holds one observation per grid cell so a vessel that keeps
// reporting positions within the same cell is served from memory instead
// of from the weather API. It is safe for concurrent use: entries are
// written by the goroutine doing the fetch and read by the mapper's own
// goroutine.
type weatherCache struct {
	mutex      sync.Mutex
	resolution float64
	maxAge     time.Duration
	maxEntries int
	entries    map[string]*weatherCacheEntry
}

func newWeatherCache(resolution float64, maxAge time.Duration, maxEntries int) *weatherCache {
	return &weatherCache{
		resolution: resolution,
		maxAge:     maxAge,
		maxEntries: maxEntries,
		entries:    make(map[string]*weatherCacheEntry),
	}
}

// key is the identity of the grid cell a position falls in. Weather is
// fetched, and reused, per cell: with the default resolution of 0.1 deg
// that is a cell of roughly 11 km of latitude, well inside the resolution
// of the weather models behind the public APIs.
func (c *weatherCache) key(latitude, longitude float64) string {
	return fmt.Sprintf(
		"%d/%d",
		int64(math.Round(latitude/c.resolution)),
		int64(math.Round(longitude/c.resolution)),
	)
}

// get returns the entry for the cell containing this position, if there is
// one that has not aged past maxAge.
func (c *weatherCache) get(latitude, longitude float64, now time.Time) (*weatherCacheEntry, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry, ok := c.entries[c.key(latitude, longitude)]
	if !ok || now.Sub(entry.fetched) > c.maxAge {
		return nil, false
	}
	return entry, true
}

func (c *weatherCache) put(latitude, longitude float64, now time.Time, observation *weatherObservation) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.entries[c.key(latitude, longitude)] = &weatherCacheEntry{observation: observation, fetched: now}
	c.evict(now)
}

// evict drops everything that aged past maxAge and, if that still leaves
// more entries than maxEntries, the oldest ones until it fits. The cache
// is small (a cell per default is 11 km wide, a vessel at 20 knots enters
// a new one every 18 minutes) so a linear scan is cheaper than maintaining
// a separate ordering. The caller holds the mutex.
func (c *weatherCache) evict(now time.Time) {
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
