package mapper

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type CacheMapper struct {
	cache     *bigcache.BigCache
	heartbeat uint64
}

func NewCacheMapper(c config.CacheConfig) (*CacheMapper, error) {
	cacheConfig := bigcache.DefaultConfig(time.Duration(c.LifeWindow) * time.Second)
	cacheConfig.HardMaxCacheSize = c.HardMaxCacheSize
	cache, err := bigcache.NewBigCache(cacheConfig)
	if err != nil {
		return nil, err
	}
	return &CacheMapper{cache: cache, heartbeat: c.Heartbeat}, nil
}

func (m *CacheMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	go m.heartBeat(publisher)

	toUpdate := &message.Mapped{}
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, toUpdate); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}

		toSend := m.update(toUpdate)
		for _, s := range toSend {
			publisher.Send(s)
		}
	}
}

func (m *CacheMapper) update(toUpdate *message.Mapped) [][]byte {
	updated := make([][]byte, 0)
	for _, u := range toUpdate.Updates {
		for _, v := range u.Values {
			changedUpdate := message.NewUpdate().WithSource(&u.Source).WithTimestamp(u.Timestamp)
			changedUpdate.AddValue(&v)
			changed := message.NewMapped().WithContext(toUpdate.Context).WithOrigin(toUpdate.Origin)
			changed.AddUpdate(changedUpdate)

			bytesChanged, err := json.Marshal(changed)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not marshal the value",
					zap.String("Error", err.Error()),
				)
				continue
			}
			cacheKey := createKey(toUpdate.Origin, toUpdate.Context, v.Path)

			// there is no entry in the cache yet or the value has changed
			if cachedValue, err := m.cache.Get(cacheKey); err != nil || equals(cachedValue, changed) {
				m.cache.Set(cacheKey, bytesChanged)
				updated = append(updated, bytesChanged)
				continue
			}
		}
	}
	return updated
}

func (m *CacheMapper) heartBeat(publisher mangos.Socket) {
	ticker := time.NewTicker(time.Duration(m.heartbeat) * time.Second)
	for range ticker.C {
		i := m.cache.Iterator()
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

func (m *CacheMapper) sendIfNeeded(publisher mangos.Socket, entry bigcache.EntryInfo) {
	now := time.Now()
	if uint64(now.Unix())-entry.Timestamp() > m.heartbeat {
		toSend, err := updateTime(entry.Value(), now)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not update the time",
				zap.String("Error", err.Error()),
				zap.ByteString("Value", entry.Value()),
			)
		}
		publisher.Send(toSend)
	}
}

func createKey(origin string, context string, path string) string {
	return strings.Join([]string{origin, context, path}, "###")
}

func equals(cached []byte, toCheck *message.Mapped) bool {
	var cachedMapped *message.Mapped
	if err := json.Unmarshal(cached, cachedMapped); err != nil {
		return false
	}

	// assume one update with one value
	if len(cachedMapped.Updates) != 1 || len(cachedMapped.Updates[0].Values) != 1 {
		return false
	}
	if len(toCheck.Updates) != 1 || len(toCheck.Updates[0].Values) != 1 {
		return false
	}

	result := true
	result = result && cachedMapped.Updates[0].Source == toCheck.Updates[0].Source
	result = result && cachedMapped.Updates[0].Values[0].Path == toCheck.Updates[0].Values[0].Path
	result = result && cachedMapped.Updates[0].Values[0].Value == toCheck.Updates[0].Values[0].Value

	return result
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
