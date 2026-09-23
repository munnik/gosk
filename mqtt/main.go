package mqtt

import (
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

const (
	keepAlive      = 30 * time.Second
	disconnectWait = 5 * time.Second

	// initialConnectWait is how long New waits for the first connection
	// before handing back a client that is still connecting. Short enough to
	// stay well inside gosk.nix's 40s TimeoutStartSec, since a processor that
	// reads from MQTT cannot publish - and so cannot report itself ready -
	// until it has connected.
	initialConnectWait = 10 * time.Second
	// connectRetryInterval is how often paho retries a connection that has
	// not been established yet.
	connectRetryInterval = 5 * time.Second
)

type Client struct {
	config         *config.MQTTConfig
	publishHandler paho.MessageHandler
	topic          string
	pahoClient     *paho.Client
}

func New(config *config.MQTTConfig, publishHandler paho.MessageHandler, topic string) *Client {
	result := &Client{
		config:         config,
		publishHandler: publishHandler,
		topic:          topic,
	}

	pahoClient := paho.NewClient(result.createClientOptions())

	// A broker that is not up yet is not a reason to die. This used to be
	// Fatal, and restarting the broker was enough to kill anything that
	// happened to start during the gap: deploying the mosquitto config to
	// hetzner-prod01 restarted it, gosk-transferRequest2 started inside that
	// window, exited 1, and took the whole activation down with it - systemd
	// restarted the unit successfully five seconds later, but by then
	// switch-to-configuration had already recorded the failure and rolled
	// back. The same Fatal on hetzner-otap01's reader made the two servers
	// deadlock, since that reader needs a broker user that only the prod01
	// deploy creates.
	//
	// SetConnectRetry has paho keep trying in the background, and
	// onConnectHandler subscribes on every connection, so a client handed
	// back before the broker is up still ends up connected and subscribed.
	token := pahoClient.Connect()
	if !token.WaitTimeout(initialConnectWait) {
		logger.GetLogger().Warn(
			"Not connected to the MQTT broker yet, continuing while it is retried in the background",
			zap.String("URL", config.URLString),
			zap.Duration("Waited", initialConnectWait),
		)
	} else if err := token.Error(); err != nil {
		logger.GetLogger().Warn(
			"Could not connect to the MQTT broker, it will be retried in the background",
			zap.String("Error", err.Error()),
			zap.String("URL", config.URLString),
		)
	}
	result.pahoClient = &pahoClient

	return result
}

func (c *Client) Publish(topic string, qos byte, retained bool, bytes []byte) {
	if token := (*c.pahoClient).Publish(topic, qos, retained, bytes); token.Wait() && token.Error() != nil {
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

func (c *Client) createClientOptions() *paho.ClientOptions {
	result := paho.NewClientOptions()
	result.AddBroker(c.config.URLString)
	result.SetUsername(c.config.Username)
	result.SetPassword(c.config.Password)

	result.SetOrderMatters(false)
	result.SetKeepAlive(keepAlive)
	result.SetAutoReconnect(true)
	// Retry the first connection too, not just reconnections after a drop.
	result.SetConnectRetry(true)
	result.SetConnectRetryInterval(connectRetryInterval)

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
		// Not Fatal: this runs on every connection, so a subscribe that fails
		// is retried the next time the client reconnects rather than killing
		// a process that is otherwise working.
		logger.GetLogger().Warn(
			"Could not subscribe to the MQTT topic, it will be retried on the next connection",
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
