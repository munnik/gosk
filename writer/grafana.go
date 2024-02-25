package writer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

const (
	publishTopic   = "grafana/%s/%s"
	bufferCapacity = 5000
)

type GrafanaWriter struct {
	mqttConfig *config.MQTTConfig
	mqttClient *mqtt.Client
}

func NewGrafanaWriter(c *config.MQTTConfig) *GrafanaWriter {
	w := &GrafanaWriter{mqttConfig: c}
	return w
}

func (w *GrafanaWriter) sendMQTT(delta *message.Mapped) {
	for _, svm := range delta.ToSingleValueMapped() {
		value, err := json.Marshal(svm.Value)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not marshal value",
				zap.String("Error", err.Error()),
			)
			continue
		}
		id := strings.ReplaceAll(svm.Context, ":", "_")
		topic := strings.ReplaceAll(fmt.Sprintf(publishTopic, id, svm.Path), ".", "/")
		w.mqttClient.Publish(topic, 0, true, value)
	}
}

func (w *GrafanaWriter) WriteMapped(subscriber *nanomsg.Subscriber[message.Mapped]) {
	w.mqttClient = mqtt.New(w.mqttConfig, nil, "")
	defer w.mqttClient.Disconnect()

	receiveBuffer := make(chan *message.Mapped, bufferCapacity)
	go subscriber.Receive(receiveBuffer)

	for mapped := range receiveBuffer {
		w.sendMQTT(mapped)
	}
}
