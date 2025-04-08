package reader

import (
	"encoding/json"
	"sync"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/gosk/nanomsg"
)

const (
	mqttTopic      = "vessels/#"
	bufferCapacity = 5000
)

type MqttReader struct {
	mqttConfig                     *config.MQTTConfig
	sendBuffer                     chan *message.Mapped
	decoder                        *zstd.Decoder
	mqttMessagesReceived           prometheus.Counter
	mqttMessagesDecompressed       prometheus.Counter
	mqttMessagesUnmarshalled       prometheus.Counter
	mqttTotalUpdatesSent           prometheus.Counter
	mqttTransferRequestUpdatesSent prometheus.Counter
}

func NewMqttReader(c *config.MQTTConfig) *MqttReader {
	decoder, _ := zstd.NewReader(nil)

	return &MqttReader{
		mqttConfig:                     c,
		decoder:                        decoder,
		mqttMessagesReceived:           promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_received_total", Help: "total number of received mqtt messages"}),
		mqttMessagesDecompressed:       promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_decompressed_total", Help: "total number of decompressed mqtt messages"}),
		mqttMessagesUnmarshalled:       promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_messages_unmarshalled_total", Help: "total number of unmarshalled mqtt messages"}),
		mqttTotalUpdatesSent:           promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_updates_sent_total", Help: "total number of updates sent"}),
		mqttTransferRequestUpdatesSent: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_mqtt_updates_sent_transfer_request", Help: "number of updates sent via a transfer request"}),
	}
}

func (r *MqttReader) ReadMapped(publisher *nanomsg.Publisher[message.Mapped]) {
	r.sendBuffer = make(chan *message.Mapped, bufferCapacity)
	defer close(r.sendBuffer)
	go publisher.Send(r.sendBuffer)

	m := mqtt.New(r.mqttConfig, r.messageHandler, mqttTopic)
	defer m.Disconnect()

	// never exit
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}

func (r *MqttReader) messageHandler(c paho.Client, m paho.Message) {
	r.mqttMessagesReceived.Inc()
	var received []byte
	var err error
	if r.mqttConfig.Compress {
		received, err = r.decoder.DecodeAll(m.Payload(), nil)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not decompress payload",
				zap.String("Error", err.Error()),
				zap.ByteString("Bytes", m.Payload()),
			)
			return
		}
		r.mqttMessagesDecompressed.Inc()
	} else {
		received = m.Payload()
	}

	messages := make([]*message.Mapped, 0)
	if err := json.Unmarshal(received, &messages); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", received),
		)
		return
	}
	r.mqttMessagesUnmarshalled.Inc()

	for _, message := range messages {
		r.sendBuffer <- message

		for _, update := range message.Updates {
			if update.Source.TransferUuid != uuid.Nil {
				r.mqttTransferRequestUpdatesSent.Inc()
			}
			r.mqttTotalUpdatesSent.Inc()
		}
	}
}
