package mapper

import (
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
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

func (r *RateLimitFilter) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, r, true)
}

func (r *RateLimitFilter) DoMap(m *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.Context).WithOrigin(m.Origin)

	// lookup context
	pathMap, present := r.lastSeen[m.Context]
	if !present {
		r.lastSeen[m.Context] = make(map[string]time.Time)
		pathMap = r.lastSeen[m.Context]
	}

	for _, svm := range m.ToSingleValueMapped() {
		// never filter empty paths
		// todo: needs better fix
		if svm.Path == "" {
			for _, u := range svm.ToMapped().Updates {
				result.AddUpdate(&u)
			}
			continue
		}

		timestamp, present := pathMap[svm.Path]
		if !present { // this path is never seen before so add to the filteredDelta
			pathMap[svm.Path] = svm.Timestamp
			for _, u := range svm.ToMapped().Updates {
				result.AddUpdate(&u)
			}
			continue
		}

		interval := r.config.DefaultInterval
		if pathInterval, present := r.rateLimit[svm.Path]; present {
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
