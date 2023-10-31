package writer

import (
	"encoding/json"
	"fmt"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	// disconnectWait = 5000 // time to wait before disconnect in ms
	// keepAlive      = 30 * time.Second
	publishTopic = "grafana/%s/%s"
)

type GrafanaWriter struct {
	mqttConfig *config.MQTTConfig
	mqttClient *mqtt.Client
}

func NewGrafanaWriter(c *config.MQTTConfig) *GrafanaWriter {
	w := &GrafanaWriter{mqttConfig: c}
	return w
}

func (w *GrafanaWriter) sendMQTT(delta message.Mapped) {
	for _, svm := range delta.ToSingleValueMapped() {
		value, err := json.Marshal(svm.Value)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not marshal value",
				zap.String("Error", err.Error()),
			)
			continue
		}
		topic := fmt.Sprintf(publishTopic, svm.Context, svm.Path)
		w.mqttClient.Publish(topic, 0, true, value)

	}

}

func (w *GrafanaWriter) WriteMapped(subscriber mangos.Socket) {
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
		var m message.Mapped
		if err := json.Unmarshal(received, &m); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.sendMQTT(m)
	}
}
