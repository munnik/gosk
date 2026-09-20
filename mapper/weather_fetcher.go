package mapper

import (
	"context"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

// sourceFetcher couples one upstream endpoint with the cache and the rate
// limiting around it. The weather mapper has one per endpoint (the
// forecast API and the marine API), which keeps a marine API that is
// down, or that has nothing to say about the canal the vessel is on, from
// affecting the atmospheric values at all.
//
// It is generic over the observation type so both endpoints share the
// caching, refreshing, backoff and single-flight logic instead of having
// it written twice.
type sourceFetcher[T observation] struct {
	// name identifies the endpoint in log messages.
	name   string
	source source[T]
	cache  *weatherCache[T]

	// refreshInterval is how long an observation is reused before a new
	// one is fetched for the same grid cell.
	refreshInterval time.Duration
	// emptyRefreshInterval is the same for an answer the provider had
	// nothing in: the marine API answers with nulls everywhere for a
	// position on an inland waterway, and a canal does not grow a sea
	// state in ten minutes, so that answer is kept far longer than a real
	// one. Without this a barge on the Rhine would spend a request per
	// refresh interval, for its whole working life, confirming that the
	// Rhine still has no swell.
	emptyRefreshInterval time.Duration
	// minRequestInterval is the hard floor between two requests
	// regardless of position, and the first delay of the backoff after a
	// failed request.
	minRequestInterval time.Duration
	requestTimeout     time.Duration

	// mutex guards the fields below, which are shared with the goroutine
	// that performs a request.
	mutex       sync.Mutex
	fetching    bool
	nextRequest time.Time
	backoff     *backoff.Backoff
}

func newSourceFetcher[T observation](name string, s source[T], cache *weatherCache[T], refreshInterval time.Duration, emptyRefreshInterval time.Duration, minRequestInterval time.Duration, maxRequestInterval time.Duration, requestTimeout time.Duration) *sourceFetcher[T] {
	return &sourceFetcher[T]{
		name:                 name,
		source:               s,
		cache:                cache,
		refreshInterval:      refreshInterval,
		emptyRefreshInterval: emptyRefreshInterval,
		minRequestInterval:   minRequestInterval,
		requestTimeout:       requestTimeout,
		backoff: &backoff.Backoff{
			Min:    minRequestInterval,
			Max:    maxRequestInterval,
			Factor: 2,
			Jitter: true,
		},
	}
}

// observation returns the cached observation for this position, if there
// is one that has not aged past the cache's maximum data age.
func (f *sourceFetcher[T]) observation(now time.Time, latitude float64, longitude float64) (T, bool) {
	entry, ok := f.cache.get(latitude, longitude, now)
	if !ok {
		var zero T
		return zero, false
	}
	return entry.observation, true
}

// request starts a request for this position when the cache cannot answer
// it, at most one at a time and never sooner than the backoff and
// minRequestInterval allow. It returns immediately, the request itself
// runs on its own goroutine: the mapper's refreshMap runs on the same
// goroutine as the rest of the pipeline's processing, blocking it on a
// network request would stall every message behind it.
func (f *sourceFetcher[T]) request(now time.Time, latitude float64, longitude float64) {
	if entry, ok := f.cache.get(latitude, longitude, now); ok {
		interval := f.refreshInterval
		if entry.observation.isEmpty() {
			interval = f.emptyRefreshInterval
		}
		if now.Sub(entry.fetched) < interval {
			// what is cached for the grid cell the vessel is in is still
			// fresh enough, this is what keeps a vessel that publishes
			// every 10 seconds from making a request every 10 seconds
			return
		}
	}

	f.mutex.Lock()
	if f.fetching || now.Before(f.nextRequest) {
		f.mutex.Unlock()
		return
	}
	f.fetching = true
	f.mutex.Unlock()

	go f.fetch(now, latitude, longitude)
}

// fetch performs a single request and caches its result. now is the moment
// the request was started, which is what the entry's age is measured from.
func (f *sourceFetcher[T]) fetch(now time.Time, latitude float64, longitude float64) {
	ctx, cancel := context.WithTimeout(context.Background(), f.requestTimeout)
	defer cancel()

	result, err := f.source.fetch(ctx, latitude, longitude)
	if err == nil {
		f.cache.put(latitude, longitude, now, result)
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.fetching = false
	if err != nil {
		// back off exponentially, an API that is down, rate limiting us
		// or rejecting our requests should not be asked again every
		// minRequestInterval for as long as the vessel is under way
		wait := f.backoff.Duration()
		f.nextRequest = now.Add(wait)
		logger.GetLogger().Warn(
			"Could not get the weather for the current position",
			zap.String("Source", f.name),
			zap.Float64("Latitude", latitude),
			zap.Float64("Longitude", longitude),
			zap.Duration("Retrying in", wait),
			zap.String("Error", err.Error()),
		)
		return
	}
	f.backoff.Reset()
	f.nextRequest = now.Add(f.minRequestInterval)
}

// idle reports whether no request is in flight, it exists for the tests.
func (f *sourceFetcher[T]) idle() bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return !f.fetching
}
