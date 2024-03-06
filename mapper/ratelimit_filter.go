package mapper

import (
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

type pathMap map[string]time.Time
type lastSeen map[string]pathMap
type rateLimit map[string]time.Duration

func (l *lastSeen) Update(context, path string, timestamp time.Time, interval time.Duration) bool {
	if _, ok := (*l)[context]; !ok {
		(*l)[context] = make(pathMap)
	}

	if _, ok := (*l)[context][path]; !ok {
		// never seen before
		(*l)[context][path] = timestamp
		return true
	}

	if timestamp.After((*l)[context][path].Add(interval)) {
		// seen long enough ago
		(*l)[context][path] = timestamp
		return true
	}

	return false
}

func (r *rateLimit) GetRateLimit(path string, def time.Duration) time.Duration {
	if d, ok := (*r)[path]; ok {
		return d
	}

	return def
}

type RateLimitFilter struct {
	config    *config.RateLimitFilterConfig
	lastSeen  lastSeen
	rateLimit rateLimit
}

func NewRateLimitFilter(c *config.RateLimitFilterConfig) (*RateLimitFilter, error) {
	rateLimit := make(rateLimit)
	for _, mapping := range c.Ratelimits {
		rateLimit[mapping.Path] = mapping.Interval
	}
	return &RateLimitFilter{config: c, lastSeen: make(lastSeen, 0), rateLimit: rateLimit}, nil
}

func (r *RateLimitFilter) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, r, true)
}

func (r *RateLimitFilter) DoMap(m *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.Context).WithOrigin(m.Origin)

	for _, svm := range m.ToSingleValueMapped() {
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

		interval := r.rateLimit.GetRateLimit(path, r.config.DefaultInterval)
		if r.lastSeen.Update(m.Context, path, svm.Timestamp, interval) {
			for _, u := range svm.ToMapped().Updates {
				result.AddUpdate(&u)
			}
			continue
		}
	}

	return result, nil
}
