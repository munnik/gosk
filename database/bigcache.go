package database

import (
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
	rawCache, _ := bigcache.NewBigCache(cacheConfig)
	mappedCache, _ := bigcache.NewBigCache(cacheConfig)
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

func (c *BigCache) WriteMapped(mappedList ...message.Mapped) []message.Mapped {
	changes := make([]message.Mapped, 0)
	for _, mapped := range mappedList {
		for _, u := range mapped.Updates {
			for _, v := range u.Values {
				singleValueUpdate := message.NewUpdate().WithSource(&u.Source).WithTimestamp(u.Timestamp)
				singleValueUpdate.AddValue(&v)
				singleUpdateMapped := message.NewMapped().WithContext(mapped.Context).WithOrigin(mapped.Origin)
				singleUpdateMapped.AddUpdate(singleValueUpdate)

				bytes, err := json.Marshal(singleUpdateMapped)
				if err != nil {
					logger.GetLogger().Warn(
						"Could not marshal the value",
						zap.String("Error", err.Error()),
					)
					continue
				}
				if originalBytes, err := c.mappedCache.Get(mapped.Context + "." + v.Path); err == nil {
					var original *message.Mapped
					if err := json.Unmarshal(originalBytes, original); err == nil {
						if original.Equals(*singleUpdateMapped) || singleValueUpdate.Timestamp.Before(original.Updates[0].Timestamp) {
							continue
						}
					}
				}
				changes = append(changes, *singleUpdateMapped)
				c.mappedCache.Set(mapped.Context+"."+v.Path, bytes)
			}
		}
	}
	return changes
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

func (c *BigCache) ReadMapped(where string, arguments ...interface{}) ([]message.Mapped, error) {
	result := make([]message.Mapped, 0)

	i := c.mappedCache.Iterator()
	for i.SetNext() {
		var mapped message.Mapped
		entry, err := i.Value()
		if err != nil {
			logger.GetLogger().Warn(
				"Error while iterating over cache",
				zap.String("Error", err.Error()),
			)
		}
		if err := json.Unmarshal(entry.Value(), &mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the value",
				zap.String("Error", err.Error()),
				zap.ByteString("Bytes", entry.Value()),
			)
			return nil, err
		}
		result = append(result, mapped)
	}
	return result, nil
}

func (c *BigCache) RawIterator() *bigcache.EntryInfoIterator {
	return c.rawCache.Iterator()
}

func (c *BigCache) MappedIterator() *bigcache.EntryInfoIterator {
	return c.mappedCache.Iterator()
}
