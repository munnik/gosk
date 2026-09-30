package connector

import (
	"fmt"
	"sync"

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
	lock       *sync.Mutex
}

func NewMQTTConnector(c *config.ConnectorConfig, mqttC *config.MQTTConfig) (*MQTTConnector, error) {
	m := MQTTConnector{
		config:     c,
		mqttConfig: mqttC,
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
	// mqtt.New waits for the first connection before returning (see mqtt's
	// initialConnectWait), so connecting inline delayed process - and with
	// it this unit reporting itself started, and its first status report
	// about the broker - by ten seconds whenever the broker was down. Let
	// it connect in the background: it retries there anyway, and
	// subscribes on every connection, so a broker that comes up later is
	// picked up either way.
	go func() {
		client := mqtt.New(m.mqttConfig, m.handleMessageReceived(stream), m.mqttConfig.Topic)
		m.lock.Lock()
		defer m.lock.Unlock()
		m.mqttClient = client
	}()
	process(stream, m.config.Name, m.config.Protocol, publisher, m.config.Timeout)
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
