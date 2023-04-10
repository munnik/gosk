package filter

import (
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
)

type MappedRateLimiter struct {
	config    *config.RateLimitConfig
	lastSeen  map[string]map[string]time.Time
	rateLimit map[string]time.Duration
}

func NewMappedRateLimiter(c *config.RateLimitConfig) (*MappedRateLimiter, error) {
	rateLimit := make(map[string]time.Duration)
	for _, mapping := range c.Ratelimits {
		rateLimit[mapping.Path] = mapping.Interval
	}
	return &MappedRateLimiter{config: c, lastSeen: make(map[string]map[string]time.Time, 0), rateLimit: rateLimit}, nil
}

func (m *MappedRateLimiter) isBlocked(delta *message.Mapped) bool {
	// lookup context
	pathMap, present := m.lastSeen[delta.Context]
	if !present {
		m.lastSeen[delta.Context] = make(map[string]time.Time)
		pathMap = m.lastSeen[delta.Context]
	}

	var blocked bool = true
	for _, svm := range delta.ToSingleValueMapped() {
		timestamp, present := pathMap[svm.Path]
		if !present { // this path is never seen before so don't block
			pathMap[svm.Path] = svm.Timestamp
			blocked = false
			continue
		}

		interval := m.config.DefaultInterval
		if pathInterval, present := m.rateLimit[svm.Path]; present {
			interval = pathInterval // a path specific interval is configured so override the default interval
		}

		if timestamp.Before(svm.Timestamp.Add(-interval)) { // last time seen is more than the configured interval so don't block
			blocked = false
		}
	}

	if !blocked {
		for _, svm := range delta.ToSingleValueMapped() { // because this delta is not blocked, update the timestamp for all paths in this delta
			m.lastSeen[svm.Context][svm.Path] = svm.Timestamp
		}
	}

	return blocked
}
