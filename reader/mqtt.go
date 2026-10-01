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
	"github.com/munnik/gosk/sdnotify"
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

	// Reaching here is what "started" means for this reader, so say so
	// before connecting rather than leaving it to nanomsg.Publisher's
	// first successful send (see pub.go's send). That send needs a vessel
	// to have actually published something on mqttTopic, and for it to
	// decompress, unmarshal and publish - none of which this process
	// controls - and mqtt.New below blocks for up to
	// mqtt.initialConnectWait first, so on a quiet fleet or a broker that
	// has just been restarted the unit could sit below gosk.nix's
	// TimeoutStartSec (40s) and be killed while doing nothing wrong. A
	// broker that is not up yet is not this process's health either; paho
	// retries it in the background and subscribes on every connection.
	sdnotify.Ready()

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
