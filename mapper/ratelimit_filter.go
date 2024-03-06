package mapper

import (
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
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

func (r *RateLimitFilter) DoMap(delta *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(delta.Context).WithOrigin(delta.Origin)

	// lookup context
	pathMap, present := r.lastSeen[delta.Context]
	if !present {
		r.lastSeen[delta.Context] = make(map[string]time.Time)
		pathMap = r.lastSeen[delta.Context]
	}

	for _, svm := range delta.ToSingleValueMapped() {
		path := svm.Path
		if path == "" {
			switch v := svm.Value.(type) {
			case message.VesselInfo:
				if v.MMSI == nil {
					path = "name"
				} else {
					path = "mmsi"
				}
			default:
				logger.GetLogger().Error("unexpected empty path",
					zap.Time("time", svm.Timestamp),
					zap.String("origin", svm.Origin),
					zap.String("context", svm.Context),
					zap.Any("value", svm.Value))
			}
		}

		timestamp, present := pathMap[path]
		if !present { // this path is never seen before so add to the filteredDelta
			pathMap[path] = svm.Timestamp
			for _, u := range svm.ToMapped().Updates {
				result.AddUpdate(&u)
			}
			continue
		}

		interval := r.config.DefaultInterval
		if pathInterval, present := r.rateLimit[path]; present {
			interval = pathInterval // a path specific interval is configured so override the default interval
		}

		if timestamp.Before(svm.Timestamp.Add(-interval)) { // last time seen is more than the configured interval so add to the filteredDelta
			pathMap[path] = svm.Timestamp
			for _, u := range svm.ToMapped().Updates {
				result.AddUpdate(&u)
			}
			continue
		}
	}

	return result, nil
}
