package ratelimit

import (
	"encoding/json"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type MappedRateLimiter struct {
	config       *config.RateLimitConfig
	timestampMap map[string]map[string]time.Time
	frequencyMap map[string]time.Duration
}

func NewMappedRateLimiter(c *config.RateLimitConfig) (*MappedRateLimiter, error) {
	frequencyMap := make(map[string]time.Duration)
	for _, mapping := range c.Ratelimits {
		frequencyMap[mapping.Path] = mapping.Interval
	}
	return &MappedRateLimiter{config: c, timestampMap: make(map[string]map[string]time.Time, 0), frequencyMap: frequencyMap}, nil
}

func (m *MappedRateLimiter) RateLimit(subscriber mangos.Socket, publisher mangos.Socket) {
	in := &message.Mapped{}
	var bytes []byte
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, in); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		var forward bool = false // check if one of the values in this delta is too old
		for _, svm := range in.ToSingleValueMapped() {
			forward = forward || m.doForward(svm)
		}

		if forward {
			for _, svm := range in.ToSingleValueMapped() { // update the timestamp for all other paths in this delta
				m.timestampMap[svm.Context][svm.Path] = svm.Timestamp
			}
			if err := publisher.Send(received); err != nil { // republish the original delta
				logger.GetLogger().Warn(
					"Unable to send the message using NanoMSG",
					zap.ByteString("Message", bytes),
					zap.String("Error", err.Error()),
				)
				continue
			}
		}
	}
}

func (m *MappedRateLimiter) doForward(in message.SingleValueMapped) bool {
	//lookup context
	pathMap, present := m.timestampMap[in.Context]
	if !present {
		m.timestampMap[in.Context] = make(map[string]time.Time)
		pathMap = m.timestampMap[in.Context]
	}
	timestamp, present := pathMap[in.Path]
	if !present {
		pathMap[in.Path] = in.Timestamp
		return true
	}
	interval := m.config.DefaultInterval
	if pathInterval, present := m.frequencyMap[in.Path]; present {
		interval = pathInterval
	}
	if timestamp.Before(in.Timestamp.Add(-interval)) {
		return true
	} else {
		return false
	}
}