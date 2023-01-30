package writer

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	disconnectWait = 5000 // time to wait before disconnect in ms
	keepAlive      = 30 * time.Second
	writeTopic     = "vessels/urn:mrn:imo:mmsi:%s"
)

type MqttWriter struct {
	mqttConfig *config.MQTTConfig
	mqttClient *mqtt.Client
	cache      *bigcache.BigCache
	encoder    *zstd.Encoder
}

func NewMqttWriter(c *config.MQTTConfig) *MqttWriter {
	w := &MqttWriter{mqttConfig: c}
	cacheConfig := bigcache.DefaultConfig(time.Duration(c.Interval) * time.Second)
	cacheConfig.HardMaxCacheSize = c.HardMaxCacheSize
	cacheConfig.OnRemove = w.onRemove
	cache, _ := bigcache.NewBigCache(cacheConfig)
	w.cache = cache
	encoder, _ := zstd.NewWriter(nil)
	w.encoder = encoder
	return w
}

// When this method is called either the interval to flush or the maximum cache size is reached
func (w *MqttWriter) onRemove(key string, entry []byte) {
	go func(entry []byte) {
		var m message.Mapped
		if err := json.Unmarshal(entry, &m); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal a message from the publisher",
				zap.String("Error", err.Error()),
			)
			return
		}

		deltas := make([]message.Mapped, 0)
		deltas = append(deltas, m)

		i := w.cache.Iterator()
		for i.SetNext() {
			var m message.Mapped
			entryInfo, err := i.Value()
			if err != nil {
				logger.GetLogger().Warn(
					"Unable to retrieve an entry from the cache",
					zap.String("Error", err.Error()),
				)
				continue
			}
			if err := json.Unmarshal(entryInfo.Value(), &m); err != nil {
				logger.GetLogger().Warn(
					"Could not unmarshal a message from the publisher",
					zap.String("Error", err.Error()),
				)
				continue
			}
			deltas = append(deltas, m)
		}

		w.sendMQTT(deltas)
		if err := w.cache.Reset(); err != nil {
			logger.GetLogger().Warn(
				"Error while resetting the cache",
				zap.String("Error", err.Error()),
			)
		}
	}(entry)
}

func (w *MqttWriter) sendMQTT(deltas []message.Mapped) {
	bytes, err := json.Marshal(deltas)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshall the deltas",
			zap.String("Error", err.Error()),
		)
		return
	}
	go func(context string, bytes []byte) {
		compressed := w.encoder.EncodeAll(bytes, make([]byte, 0, len(bytes)))
		w.mqttClient.Publish(context, 0, true, compressed)
	}(fmt.Sprintf(writeTopic, w.mqttConfig.Username), bytes)
}

func (w *MqttWriter) WriteMapped(subscriber mangos.Socket) {
	w.mqttClient = mqtt.New(w.mqttConfig, nil, "")
	defer w.mqttClient.Disconnect()

	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := w.cache.Set(uuid.NewString(), received); err != nil {
			logger.GetLogger().Warn(
				"Could not add the entry to the cache",
				zap.String("Error", err.Error()),
			)
		}
	}
}
