package mapper

import (
	"encoding/json"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type SendOnChangeMapper struct {
	heartbeat uint64
	bc        *database.BigCache
}

func NewSendOnChangeMapper(c config.CacheConfig) *SendOnChangeMapper {
	return &SendOnChangeMapper{
		heartbeat: c.Heartbeat,
		bc:        database.NewBigCache(c.BigCacheConfig),
	}
}

func (m *SendOnChangeMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	go m.heartBeat(publisher)

	mapped := &message.Mapped{}
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		for _, changed := range m.bc.WriteMapped(mapped, true) {
			bytes, err := json.Marshal(changed)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not marshal the value",
					zap.String("Error", err.Error()),
				)
				continue
			}
			if err := publisher.Send(bytes); err != nil {
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

func (m *SendOnChangeMapper) heartBeat(publisher mangos.Socket) {
	ticker := time.NewTicker(time.Duration(m.heartbeat) * time.Second)
	for range ticker.C {
		i := m.bc.MappedIterator()
		for i.SetNext() {
			entry, err := i.Value()
			if err != nil {
				logger.GetLogger().Warn(
					"Could not retrieve a value from the cache",
					zap.String("Error", err.Error()),
				)
				continue
			}

			m.sendIfNeeded(publisher, entry)
		}
	}
}

func (m *SendOnChangeMapper) sendIfNeeded(publisher mangos.Socket, entry bigcache.EntryInfo) {
	now := time.Now()
	if uint64(now.Unix())-entry.Timestamp() > m.heartbeat {
		bytes, err := updateTime(entry.Value(), now)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not update the time",
				zap.String("Error", err.Error()),
				zap.ByteString("Value", entry.Value()),
			)
		}
		if err := publisher.Send(bytes); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", bytes),
				zap.String("Error", err.Error()),
			)
		}
	}
}

func updateTime(bytes []byte, newTime time.Time) ([]byte, error) {
	var mapped *message.Mapped
	if err := json.Unmarshal(bytes, mapped); err != nil {
		return nil, err
	}

	for _, u := range mapped.Updates {
		u.Timestamp = newTime
	}

	result, err := json.Marshal(mapped)
	if err != nil {
		return nil, err
	}
	return result, nil
}
