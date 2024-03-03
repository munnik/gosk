package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

type BigCache struct {
	rawCache    *bigcache.BigCache
	mappedCache *bigcache.BigCache
}

func NewBigCache(c *config.BigCacheConfig) *BigCache {
	cacheConfig := bigcache.DefaultConfig(time.Duration(c.LifeWindow) * time.Second)
	cacheConfig.HardMaxCacheSize = c.HardMaxCacheSize
	rawCache, _ := bigcache.New(context.Background(), cacheConfig)
	mappedCache, _ := bigcache.New(context.Background(), cacheConfig)
	return &BigCache{rawCache: rawCache, mappedCache: mappedCache}
}

func (c *BigCache) WriteRaw(raw *message.Raw, returnChanges bool) []*message.Raw {
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshal the value",
			zap.String("Error", err.Error()),
		)
		return []*message.Raw{}
	}

	c.rawCache.Set(raw.Uuid.String(), rawBytes)
	if returnChanges {
		return []*message.Raw{raw}
	}

	return []*message.Raw{}
}

func (c *BigCache) WriteMapped(mappedList ...*message.Mapped) []*message.Mapped {
	changes := make([]message.SingleValueMapped, 0)
	for _, mapped := range mappedList {
		for _, m := range mapped.ToSingleValueMapped() {
			if originalBytes, err := c.mappedCache.Get(m.Context + "." + m.Path); err == nil {
				var original message.SingleValueMapped
				if err := json.Unmarshal(originalBytes, &original); err == nil {
					if original.Equals(m) || m.Timestamp.Before(original.Timestamp) {
						continue
					}
				}
				m = original.Merge(m)
			}
			changes = append(changes, m)
			bytes, err := json.Marshal(m)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not marshal the value",
					zap.String("Error", err.Error()),
				)
				continue
			}
			c.mappedCache.Set(m.Context+"."+m.Path, bytes)
		}
	}

	result := make([]*message.Mapped, 0)
	for _, c := range changes {
		result = append(result, c.ToMapped())
	}
	return result
}

func (c *BigCache) ReadRaw(where string, arguments ...interface{}) ([]message.Raw, error) {
	if uuid, err := uuid.Parse(where); err == nil {
		bytes, err := c.rawCache.Get(uuid.String())
		if err != nil {
			logger.GetLogger().Warn(
				"Could not find the raw message",
				zap.String("Key", uuid.String()),
				zap.String("Error", err.Error()),
			)
			return nil, err
		}

		var raw message.Raw
		if err := json.Unmarshal(bytes, &raw); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the value",
				zap.String("Error", err.Error()),
				zap.ByteString("Bytes", bytes),
			)
			return nil, err
		}
		return []message.Raw{raw}, nil
	} else if len(where) > 0 {
		return nil, fmt.Errorf("where parameter should be a valid uuid string")
	}

	result := make([]message.Raw, 0)
	i := c.rawCache.Iterator()
	for i.SetNext() {
		var raw message.Raw
		entry, err := i.Value()
		if err != nil {
			logger.GetLogger().Warn(
				"Error while iterating over cache",
				zap.String("Error", err.Error()),
			)
		}
		if err := json.Unmarshal(entry.Value(), &raw); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the value",
				zap.String("Error", err.Error()),
				zap.ByteString("Bytes", entry.Value()),
			)
			return nil, err
		}
		result = append(result, raw)
	}
	return result, nil
}

func (c *BigCache) ReadMapped(where string, arguments ...interface{}) ([]*message.Mapped, error) {
	result := make([]*message.Mapped, 0)

	i := c.mappedCache.Iterator()
	for i.SetNext() {
		entry, err := i.Value()
		if err != nil {
			logger.GetLogger().Warn(
				"Error while iterating over cache",
				zap.String("Error", err.Error()),
			)
		}
		var m message.SingleValueMapped
		if err := json.Unmarshal(entry.Value(), &m); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the value",
				zap.String("Error", err.Error()),
				zap.ByteString("Bytes", entry.Value()),
			)
			return nil, err
		}
		result = append(result, m.ToMapped())
	}
	return result, nil
}

func (c *BigCache) RawIterator() *bigcache.EntryInfoIterator {
	return c.rawCache.Iterator()
}

func (c *BigCache) MappedIterator() *bigcache.EntryInfoIterator {
	return c.mappedCache.Iterator()
}
