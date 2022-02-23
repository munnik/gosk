package writer

import (
	"encoding/json"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
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
)

type MqttWriter struct {
	config     *config.MQTTConfig
	mqttClient mqtt.Client
	buffer     message.Buffer
	timer      time.Timer
	encoder    *zstd.Encoder
}

func NewMqttWriter(c *config.MQTTConfig) *MqttWriter {
	var encoder, _ = zstd.NewWriter(nil)
	return &MqttWriter{
		config:  c,
		buffer:  *message.NewBuffer(),
		encoder: encoder,
	}
}

func (w *MqttWriter) createClientOptions() *mqtt.ClientOptions {
	o := mqtt.NewClientOptions()
	o.AddBroker(w.config.URLString)
	o.SetClientID(w.config.ClientId)
	o.SetCleanSession(true) // TODO: verify
	o.SetUsername(w.config.Username)
	o.SetPassword(w.config.Password)
	o.SetOrderMatters(false)
	o.SetKeepAlive(keepAlive)
	return o
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

	go func(w *MqttWriter) {
		for {
			m := message.Mapped{}
			received, err := subscriber.Recv()
			if err != nil {
				logger.GetLogger().Warn(
					"Could not receive a message from the publisher",
					zap.String("Error", err.Error()),
				)
				continue
			}
			if err := json.Unmarshal(received, &m); err != nil {
				logger.GetLogger().Warn(
					"Could not unmarshal a message from the publisher",
					zap.String("Error", err.Error()),
				)
				continue
			}
			w.buffer.Lock().Append(m).Unlock()
		}
	}(w)

	for {
		w.timer = *time.NewTimer(time.Duration(w.config.Interval) * time.Second)
		<-w.timer.C
		w.sendMqtt()
	}
}

func (w *MqttWriter) sendMqtt() error {
	w.buffer.Lock()
	defer w.buffer.Unlock()
	if len(w.buffer.Deltas) == 0 {
		return nil
	}
	bytes, err := json.Marshal(w.buffer)
	if err != nil {
		return err
	}
	w.buffer.Empty()

	go func(context string, bytes []byte) {
		compressed := w.encoder.EncodeAll(bytes, make([]byte, 0, len(bytes)))
		if token := w.mqttClient.Publish(context, 1, true, compressed); token.Wait() && token.Error() != nil {
			logger.GetLogger().Warn(
				"Could not publish a message via MQTT",
				zap.String("Error", token.Error().Error()),
				zap.ByteString("Bytes", bytes),
			)
		}
	}(w.config.Context, bytes)

	return nil
}
