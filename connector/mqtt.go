package connector

import (
	"fmt"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/gosk/nanomsg"
)

type MQTTConnector struct {
	config     *config.ConnectorConfig
	mqttConfig *config.MQTTConfig
	mqttClient *mqtt.Client
	timeout    *time.Timer
	lock       *sync.Mutex
}

func NewMQTTConnector(c *config.ConnectorConfig, mqttC *config.MQTTConfig) (*MQTTConnector, error) {
	m := MQTTConnector{
		config:     c,
		mqttConfig: mqttC,
		timeout:    time.AfterFunc(c.Timeout, exit),
		lock:       &sync.Mutex{},
	}
	if mqttC.Topic == "" {
		return nil, fmt.Errorf("Topic can't be empty")
	}
	return &m, nil
}

func (m *MQTTConnector) Publish(publisher *nanomsg.Publisher[message.Raw]) {
	stream := make(chan []byte, 1)
	defer close(stream)
	m.mqttClient = mqtt.New(m.mqttConfig, m.handleMessageReceived(stream), m.mqttConfig.Topic)
	process(stream, m.config.Name, m.config.Protocol, publisher, m.timeout, m.config.Timeout)
}

func (m *MQTTConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	// don't support writing to mqtt via the connector yet, use the writer
}
func (m *MQTTConnector) handleMessageReceived(stream chan<- []byte) paho.MessageHandler {
	return func(c paho.Client, message paho.Message) {
		stream <- message.Payload()
		// fmt.Println(message.Topic(), string(message.Payload()))

	}
}
