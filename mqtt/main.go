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
}

func New(config *config.MQTTConfig, publishHandler mqtt.MessageHandler, topic string) {
	c := Client{
		config:         config,
		publishHandler: publishHandler,
	}

	pahoClient := paho.NewClient(c.createClientOptions())
	if token := pahoClient.Connect(); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the MQTT broker",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", config.URLString),
		)
		return
	}
	defer pahoClient.Disconnect(uint(disconnectWait.Milliseconds()))
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

	if token := pahoClient.Subscribe(c.topic, 1, nil); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the MQTT topic",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", c.config.URLString),
		)
		return
	}
}

func connectionLostHandler(c paho.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}
}
