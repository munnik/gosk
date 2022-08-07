package writer

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/allegro/bigcache/v3"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	disconnectWait = 5000 // time to wait before disconnect in ms
	keepAlive      = 30 * time.Second
	writeTopic     = "vessels/urn:mrn:imo:mmsi:%s"
)

type MqttWriter struct {
	config     *config.MQTTConfig
	mqttClient mqtt.Client
	cache      *bigcache.BigCache
	encoder    *zstd.Encoder
	mu         sync.Mutex
}

func NewMqttWriter(c *config.MQTTConfig) *MqttWriter {
	w := &MqttWriter{config: c}
	cacheConfig := bigcache.DefaultConfig(time.Duration(c.Interval) * time.Second)
	cacheConfig.HardMaxCacheSize = c.HardMaxCacheSize
	cacheConfig.OnRemove = w.onRemove
	cache, _ := bigcache.NewBigCache(cacheConfig)
	w.cache = cache
	encoder, _ := zstd.NewWriter(nil)
	w.encoder = encoder
	return w
}

func (w *MqttWriter) createClientOptions() *mqtt.ClientOptions {
	o := mqtt.NewClientOptions()
	o.AddBroker(w.config.URLString)
	o.SetCleanSession(true) // TODO: verify
	o.SetUsername(w.config.Username)
	o.SetPassword(w.config.Password)
	o.SetOrderMatters(false)
	o.SetKeepAlive(keepAlive)
	o.SetConnectionLostHandler(disconnectHandler)
	return o
}

// When this method is called either the interval to flush or the maximum cache size is reached
func (w *MqttWriter) onRemove(key string, entry []byte) {
	go func(entry []byte) {
		w.mu.Lock()
		defer w.mu.Unlock()

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
		if token := w.mqttClient.Publish(context, 0, true, compressed); token.Wait() && token.Error() != nil {
			logger.GetLogger().Warn(
				"Could not publish a message via MQTT",
				zap.String("Error", token.Error().Error()),
				zap.ByteString("Bytes", bytes),
			)
		}
	}(fmt.Sprintf(writeTopic, w.config.Username), bytes)
}

func (w *MqttWriter) WriteMapped(subscriber mangos.Socket) {
	w.mqttClient = mqtt.NewClient(w.createClientOptions())
	if token := w.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the MQTT broker",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", w.config.URLString),
		)
		return
	}
	defer w.mqttClient.Disconnect(disconnectWait)

	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.mu.Lock()
		w.cache.Set(uuid.NewString(), received)
		w.mu.Unlock()
	}
}

func disconnectHandler(c mqtt.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}
}
