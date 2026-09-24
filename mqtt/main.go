package mqtt

import (
	"fmt"
	"os"
	"strings"
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
	// maxReconnectInterval caps paho's reconnect backoff. Its own default
	// is 10 minutes, which on a vessel means up to 10 minutes of publishes
	// piling up in the store after the link has already come back.
	maxReconnectInterval = 1 * time.Minute

	// publishAckTimeout is how long watchPublishTokens waits for a QoS 1
	// publish to be acknowledged before reporting it. Nothing is given up
	// on when it expires - paho keeps retrying from its store either way -
	// it only bounds how long the watcher stays on one message.
	publishAckTimeout = 2 * time.Minute
	// maxResumePubInFlight bounds how many stored publishes are re-sent at
	// once when a connection is re-established, see createClientOptions.
	maxResumePubInFlight = 100
	// maxTrackedPublishes bounds how many unacknowledged publishes are
	// waited on for logging purposes. Past that the broker is clearly not
	// keeping up, which is already being reported, and tracking more would
	// only pin the publish in memory for longer.
	maxTrackedPublishes = 256
)

type Client struct {
	config         *config.MQTTConfig
	publishHandler paho.MessageHandler
	topic          string
	pahoClient     *paho.Client
	clientID       string
	publishTokens  chan paho.Token
}

// New connects a client to the broker. role names what this process does
// with the connection ("read", "write", "transferRequest", ...); it is
// only used to derive a client id when the configuration does not set one,
// and so has to be unique among the gosk processes that run on one host
// and talk to the same broker. See MQTTConfig.ClientID.
func New(config *config.MQTTConfig, role string, publishHandler paho.MessageHandler, topic string) *Client {
	result := &Client{
		config:         config,
		publishHandler: publishHandler,
		topic:          topic,
		clientID:       clientID(config, role),
		publishTokens:  make(chan paho.Token, maxTrackedPublishes),
	}
	go result.watchPublishTokens()

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

// Publish hands a message to paho and returns without waiting for it to be
// delivered.
//
// Waiting was viable when everything published at QoS 0, where the token
// completes as soon as the packet reaches the network. At QoS 1 the token
// only completes on the broker's PUBACK, so waiting would block the caller
// for the entire length of an outage - the writer's flush, a transfer
// response - which is exactly backwards: the point of QoS 1 here is that
// paho holds on to the message and redelivers it later, on its own, while
// the caller gets on with the next one.
//
// Delivery is therefore reported asynchronously, see watchPublishTokens.
func (c *Client) Publish(topic string, qos byte, retained bool, bytes []byte) {
	token := (*c.pahoClient).Publish(topic, qos, retained, bytes)

	select {
	case c.publishTokens <- token:
	default:
		// Already tracking the maximum number of unacknowledged
		// publishes. Dropping the token only gives up on logging this
		// one's outcome; the message itself is unaffected.
	}
}

func (c *Client) watchPublishTokens() {
	for token := range c.publishTokens {
		if !token.WaitTimeout(publishAckTimeout) {
			logger.GetLogger().Warn(
				"A message published via MQTT has not been acknowledged yet, it will be redelivered until it is",
				zap.String("ClientID", c.clientID),
				zap.Duration("Waited", publishAckTimeout),
			)
			continue
		}
		if err := token.Error(); err != nil {
			logger.GetLogger().Warn(
				"Could not publish a message via MQTT",
				zap.String("ClientID", c.clientID),
				zap.Error(err),
			)
		}
	}
}

func (c *Client) Disconnect() {
	(*c.pahoClient).Disconnect(uint(disconnectWait.Milliseconds()))
}

// clientID returns the configured client id, or derives one from the
// process's role and the hostname. Both halves matter: the hostname keeps
// two vessels (or a vessel and the cloud) apart, the role keeps the
// several gosk processes on one host apart. It stays the same across
// restarts, which is what lets the broker hold a session for it.
func clientID(c *config.MQTTConfig, role string) string {
	if c.ClientID != "" {
		return c.ClientID
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		// Falling back to something non-unique would have two clients
		// disconnect each other in a loop, which is worse than not
		// starting. Refuse instead and let the operator set client_id.
		logger.GetLogger().Fatal(
			"Could not determine the hostname to derive an MQTT client id from, set client_id in the configuration",
			zap.String("Role", role),
			zap.Error(err),
		)
	}

	return sanitizeClientID(fmt.Sprintf("gosk-%s-%s", role, host))
}

// sanitizeClientID keeps the id to characters every broker accepts.
func sanitizeClientID(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, id)
}

func (c *Client) createClientOptions() *paho.ClientOptions {
	result := paho.NewClientOptions()
	result.AddBroker(c.config.URLString)
	result.SetUsername(c.config.Username)
	result.SetPassword(c.config.Password)

	result.SetOrderMatters(false)
	result.SetKeepAlive(keepAlive)
	result.SetAutoReconnect(true)
	result.SetMaxReconnectInterval(maxReconnectInterval)
	// Retry the first connection too, not just reconnections after a drop.
	result.SetConnectRetry(true)
	result.SetConnectRetryInterval(connectRetryInterval)

	// A stable client id and a session the broker is asked to keep are
	// what make QoS 1 worth anything across a reconnect. Without them the
	// broker hands out a random id per connection and throws the session
	// away the moment the link drops, so nothing is queued for a
	// subscriber that is offline and nothing unacknowledged is resumed for
	// a publisher that reconnects - which is the failure this whole
	// transfer/reconciliation machinery exists to clean up after.
	//
	// Note that the broker has to be willing to hold that session:
	// mosquitto's max_queued_messages defaults to 1000, which is far too
	// small for a vessel's worth of deltas, and persistence has to be on
	// for the queue to survive a broker restart.
	result.SetClientID(c.clientID)
	result.SetCleanSession(false)
	// Republish anything still unacknowledged from a previous run of this
	// process, rather than only what was published since it started.
	result.SetResumeSubs(true)
	// Drip the backlog out on reconnect instead of handing the whole store
	// to the network at once (paho's default is no limit). The link that
	// just came back is the same one that dropped, and a vessel that was
	// offline for a day has a backlog large enough that publishing it in
	// one burst is a good way to lose it again.
	result.SetMaxResumePubInFlight(maxResumePubInFlight)

	if c.config.StoreDir != "" {
		directory := storeDirectory(c.config.StoreDir, c.clientID)
		result.SetStore(newBoundedFileStore(directory, c.config.StoreMaxSize))
		logger.GetLogger().Info(
			"Persisting undelivered MQTT messages on disk",
			zap.String("Directory", directory),
			zap.Int64("MaxBytes", c.config.StoreMaxSize),
		)
	}

	result.SetDefaultPublishHandler(c.publishHandler)
	result.SetOnConnectHandler(c.onConnectHandler)
	result.SetConnectionLostHandler(connectionLostHandler)

	return result
}

func (c *Client) onConnectHandler(pahoClient paho.Client) {
	// The client id is logged on every connection because a duplicate one
	// is otherwise very hard to spot: two clients sharing an id take turns
	// disconnecting each other, which looks like a flaky link rather than
	// a configuration mistake.
	logger.GetLogger().Info(
		"MQTT connection established",
		zap.String("ClientID", c.clientID),
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
