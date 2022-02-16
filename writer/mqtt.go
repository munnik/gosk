package writer

import (
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	disconnectWait = 5000 // time to wait before disconnect in ms
	keepAlive      = 30 * time.Second
)

type MqttWriter struct {
	config *config.MqttConfig
}

func NewMqttWriter(c *config.MqttConfig) *MqttWriter {
	return &MqttWriter{
		config: c,
	}
}

func (w *MqttWriter) createClientOptions() *mqtt.ClientOptions {
	o := mqtt.NewClientOptions()
	o.AddBroker(w.config.URI)
	o.SetClientID(w.config.ClientId)
	o.SetCleanSession(true) // TODO: verify
	o.SetUsername(w.config.Username)
	o.SetPassword(w.config.Password)
	o.SetOrderMatters(false)
	o.SetKeepAlive(keepAlive)
	return o
}

func (w *MqttWriter) WriteMapped(subscriber mangos.Socket) {
	client := mqtt.NewClient(w.createClientOptions())
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the MQTT broker",
			zap.String("Error", token.Error().Error()),
			zap.String("Url", w.config.URI),
		)
		return
	}
	defer client.Disconnect(disconnectWait)

	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		go func(m []byte) {
			if token := client.Publish(w.config.Context, 1, true, m); token.Wait() && token.Error() != nil {
				logger.GetLogger().Warn(
					"Could not send a message to the mqtt broker",
					zap.String("Error", token.Error().Error()),
				)
			}
		}(received)
	}
}
