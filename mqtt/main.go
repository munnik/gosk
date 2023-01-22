package mqtt

import (
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

const (
	keepAlive      = 30 * time.Second
	disconnectWait = 5 * time.Second
)

type Client struct {
	config         *config.MQTTConfig
	publishHandler mqtt.MessageHandler
	topic          string
	pahoClient     *paho.Client
}

func New(config *config.MQTTConfig, publishHandler mqtt.MessageHandler, topic string) *Client {
	result := &Client{
		config:         config,
		publishHandler: publishHandler,
		topic:          topic,
	}

	pahoClient := paho.NewClient(result.createClientOptions())
	if token := pahoClient.Connect(); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the MQTT broker",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", config.URLString),
		)
		return nil
	}
	result.pahoClient = &pahoClient

	return result
}

func (c *Client) Publish(topic string, qos byte, retained bool, bytes []byte) {
	if token := (*c.pahoClient).Publish(topic, 0, true, bytes); token.Wait() && token.Error() != nil {
		logger.GetLogger().Warn(
			"Could not publish a message via MQTT",
			zap.String("Error", token.Error().Error()),
			zap.String("Topic", topic),
			zap.ByteString("Bytes", bytes),
		)
	}
}

func (c *Client) Disconnect() {
	(*c.pahoClient).Disconnect(uint(disconnectWait.Milliseconds()))
}

func (c *Client) createClientOptions() *mqtt.ClientOptions {
	result := mqtt.NewClientOptions()
	result.AddBroker(c.config.URLString)
	result.SetUsername(c.config.Username)
	result.SetPassword(c.config.Password)

	result.SetOrderMatters(false)
	result.SetKeepAlive(keepAlive)
	result.SetAutoReconnect(true)

	result.SetDefaultPublishHandler(c.publishHandler)
	result.SetOnConnectHandler(c.onConnectHandler)
	result.SetConnectionLostHandler(connectionLostHandler)

	return result
}

func (c *Client) onConnectHandler(pahoClient paho.Client) {
	logger.GetLogger().Info(
		"MQTT connection established",
	)

	if c.topic == "" {
		logger.GetLogger().Info(
			"Topic is empty so not subscribing",
			zap.String("URL", c.config.URLString),
		)
		return
	}

	if token := pahoClient.Subscribe(c.topic, 1, nil); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the MQTT topic",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", c.config.URLString),
			zap.String("Topic", c.topic),
		)
		return
	}

	logger.GetLogger().Info(
		"Subscribed to the MQTT topic",
		zap.String("URL", c.config.URLString),
		zap.String("Topic", c.topic),
	)
}

func connectionLostHandler(c paho.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}
}
