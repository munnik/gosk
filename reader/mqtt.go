package reader

import (
	"encoding/json"
	"sync"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"go.nanomsg.org/mangos/v3"
)

const (
	mqttTopic = "vessels/#"
)

// var mqttMessagesReceived =

type MqttReader struct {
	mqttConfig               *config.MQTTConfig
	publisher                mangos.Socket
	decoder                  *zstd.Decoder
	mqttMessagesReceived     prometheus.Counter
	mqttMessagesDecompressed prometheus.Counter
	mqttMessagesUnmarshalled prometheus.Counter
	mqttDeltasSent           prometheus.Counter
}

func NewMqttReader(c *config.MQTTConfig) *MqttReader {
	decoder, _ := zstd.NewReader(nil)
	return &MqttReader{
		mqttConfig:               c,
		decoder:                  decoder,
		mqttMessagesReceived:     promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_received_total", Help: "total number of received mqtt messages"}),
		mqttMessagesDecompressed: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_decompressed_total", Help: "total number of decompressed mqtt messages"}),
		mqttMessagesUnmarshalled: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_unmarshalled_total", Help: "total number of unmarshalled mqtt messages"}),
		mqttDeltasSent:           promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_deltas_sent_total", Help: "total number of deltas sent"}),
	}
}

func (r *MqttReader) ReadMapped(publisher mangos.Socket) {
	r.publisher = publisher

	m := mqtt.New(r.mqttConfig, r.messageReceived, mqttTopic)
	defer m.Disconnect()

	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (r *MqttReader) messageReceived(c paho.Client, m paho.Message) {
	r.mqttMessagesReceived.Inc()
	received, err := r.decoder.DecodeAll(m.Payload(), nil)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not decompress payload",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	r.mqttMessagesDecompressed.Inc()

	var deltas []message.Mapped
	if err := json.Unmarshal(received, &deltas); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", received),
		)
		return
	}
	r.mqttMessagesUnmarshalled.Inc()

	for _, delta := range deltas {
		bytes, err := json.Marshal(delta)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not marshal delta",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := r.publisher.Send(bytes); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
		r.mqttDeltasSent.Inc()
	}
}
