package mapper

import (
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
)

type RateLimitFilter struct {
	config    *config.RateLimitFilterConfig
	lastSeen  map[string]map[string]time.Time
	rateLimit map[string]time.Duration
}

func NewRateLimitFilter(c *config.RateLimitFilterConfig) (*RateLimitFilter, error) {
	rateLimit := make(map[string]time.Duration)
	for _, mapping := range c.Ratelimits {
		rateLimit[mapping.Path] = mapping.Interval
	}
	return &RateLimitFilter{config: c, lastSeen: make(map[string]map[string]time.Time, 0), rateLimit: rateLimit}, nil
}

func (f *RateLimitFilter) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	processMapped(subscriber, publisher, f)
}

func (m *RateLimitFilter) DoMap(delta *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(delta.Context).WithOrigin(delta.Origin)

	// lookup context
	pathMap, present := m.lastSeen[delta.Context]
	if !present {
		m.lastSeen[delta.Context] = make(map[string]time.Time)
		pathMap = m.lastSeen[delta.Context]
	}

	for _, svm := range delta.ToSingleValueMapped() {
		timestamp, present := pathMap[svm.Path]
		if !present { // this path is never seen before so add to the filteredDelta
			pathMap[svm.Path] = svm.Timestamp
			for _, u := range svm.ToMapped().Updates {
				result.AddUpdate(&u)
			}
			continue
		}

		interval := m.config.DefaultInterval
		if pathInterval, present := m.rateLimit[svm.Path]; present {
			interval = pathInterval // a path specific interval is configured so override the default interval
		}

		if timestamp.Before(svm.Timestamp.Add(-interval)) { // last time seen is more than the configured interval so add to the filteredDelta
			pathMap[svm.Path] = svm.Timestamp
			for _, u := range svm.ToMapped().Updates {
				result.AddUpdate(&u)
			}
			continue
		}
	}

	return result, nil
}
