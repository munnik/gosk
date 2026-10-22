package mqtt

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

const (
	// subackRefused is the SUBACK return code for a refused subscription.
	subackRefused byte = 0x80
	// How long retrySubscribe waits before trying a failed subscription
	// again, doubling up to the maximum.
	subscribeRetryMin = time.Second
	subscribeRetryMax = time.Minute
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
	// is 10 minutes, which on a vessel means up to 10 minutes of waiting
	// after the link has already come back.
	maxReconnectInterval = 1 * time.Minute

	// publishAckTimeout is how long watchPublishTokens waits for a QoS 1
	// publish to be acknowledged before reporting it. Nothing is given up
	// on when it expires - paho keeps retrying either way - it only bounds
	// how long the watcher stays on one message.
	publishAckTimeout = 2 * time.Minute
	// maxResumePubInFlight bounds how many stored publishes are re-sent at
	// once when a connection is re-established, see createClientOptions.
	maxResumePubInFlight = 100
	// maxTrackedPublishes bounds how many unacknowledged publishes are
	// waited on for logging purposes. Past that the broker is clearly not
	// keeping up, which is already being reported.
	maxTrackedPublishes = 256
)

// sessionRoles are the roles whose client gets a stable id and a session
// the broker keeps across connections (see createClientOptions): the
// outbox, whose acknowledgements and deliveries are QoS 1 and have to
// survive a dropped link. Every other role connects as it always has, with
// a clean session under an id the broker assigns. Not for want of a name
// but because a derived one would collide: a vessel runs its live writer
// and the writer behind its responder as two "write" processes against the
// same broker with the same username, and two clients sharing an id take
// turns disconnecting each other. A configured client_id applies to any
// role, and brings the session with it.
var sessionRoles = map[string]bool{
	"transferSend":    true,
	"transferReceive": true,
	"transferSeed":    true,
	"transferDigest":  true,
}

type Client struct {
	config         *config.MQTTConfig
	publishHandler paho.MessageHandler
	topics         []string
	pahoClient     *paho.Client
	// clientID is empty for a role without a session, see sessionRoles.
	clientID      string
	publishTokens chan paho.Token
	// subscribed is closed the first time every topic has been subscribed
	// to, for a caller that publishes a request whose answer comes back on
	// one of them (WaitSubscribed).
	subscribed     chan struct{}
	subscribedOnce sync.Once
	// connection counts connections; a retry worker belongs to the one it
	// was started on, and stops once another has replaced it.
	connection atomic.Uint64
	// session is whether the broker is asked to keep this client's session,
	// see sessionRoles.
	session bool
}

// New builds a client for the broker without connecting it; call Connect
// once everything the handler touches is in place. role names what this
// process does with the connection ("read", "write", "transferSend", ...).
// For the roles in sessionRoles it is what the client id is derived from
// when the configuration does not set one, and so has to be unique among
// the gosk processes on one host that connect to the same broker with the
// same username. See MQTTConfig.ClientID.
//
// more subscribes the client to further topics, all through the same
// handler, which tells them apart by the message's topic. One client rather
// than one per topic because sessions are persistent (createClientOptions):
// a second client the process only needs for a moment would leave the broker
// queueing for it after it is gone.
func New(config *config.MQTTConfig, role string, publishHandler paho.MessageHandler, topic string, more ...string) *Client {
	topics := []string{}
	for _, t := range append([]string{topic}, more...) {
		if t != "" {
			topics = append(topics, t)
		}
	}
	result := &Client{
		config:         config,
		publishHandler: publishHandler,
		topics:         topics,
		subscribed:     make(chan struct{}),
		clientID:       clientID(config, role),
		session:        config.ClientID != "" || sessionRoles[role],
		publishTokens:  make(chan paho.Token, maxTrackedPublishes),
	}
	pahoClient := paho.NewClient(result.createClientOptions())
	result.pahoClient = &pahoClient
	return result
}

// Connect connects to the broker and subscribes; it returns once connected
// or after a short wait, with paho retrying in the background.
//
// Connecting is separate from New because a persistent session (see
// createClientOptions) can have messages queued at the broker, which are
// delivered - and the handler called - the moment the connection is up.
// A handler that publishes a reply through the *Client its owner stores
// (the outbox receiver's acknowledgement, the digest responder's reply)
// needs that field assigned first, so: New, store it, Connect.
//
// A broker that is not up yet is not a reason to die. This used to be
// Fatal, and restarting the broker was enough to kill anything that
// happened to start during the gap: deploying the mosquitto config to
// the fleet broker's host restarted it, gosk-transferRequest2 started inside that
// window, exited 1, and took the whole activation down with it - systemd
// restarted the unit successfully five seconds later, but by then
// switch-to-configuration had already recorded the failure and rolled
// back. The same Fatal on the test server's reader made the two servers
// deadlock, since that reader needs a broker user that only the production
// deploy creates.
//
// SetConnectRetry has paho keep trying in the background, and
// onConnectHandler subscribes on every connection, so a client handed
// back before the broker is up still ends up connected and subscribed.
func (c *Client) Connect() {
	go c.watchPublishTokens()

	token := (*c.pahoClient).Connect()
	if !token.WaitTimeout(initialConnectWait) {
		logger.GetLogger().Warn(
			"Not connected to the MQTT broker yet, continuing while it is retried in the background",
			zap.String("URL", c.config.URLString),
			zap.String("ClientID", c.clientID),
			zap.Duration("Waited", initialConnectWait),
		)
	} else if err := token.Error(); err != nil {
		logger.GetLogger().Warn(
			"Could not connect to the MQTT broker, it will be retried in the background",
			zap.String("Error", err.Error()),
			zap.String("URL", c.config.URLString),
			zap.String("ClientID", c.clientID),
		)
	}
}

// Publish hands a message to paho and returns without waiting for it to be
// delivered.
//
// Waiting was viable when everything published at QoS 0, where the token
// completes as soon as the packet reaches the network. At QoS 1 the token
// only completes on the broker's PUBACK, so waiting would block the caller
// for the entire length of an outage - which is exactly backwards: the
// point of QoS 1 is that paho holds on to the message and redelivers it
// later, on its own, while the caller gets on with the next one. Delivery
// is therefore reported asynchronously, see watchPublishTokens.
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

// Connected reports whether the connection to the broker is up right
// now - not whether paho is trying, which is what its IsConnected answers
// while it reconnects. A publisher with a backlog of its own (the outbox
// sender) checks this before handing paho more, since what paho takes
// while the link is down it keeps in memory.
// WaitSubscribed blocks until the client has been subscribed to all its
// topics at least once, or timeout passes, and reports which. A request
// published before then can have its answer delivered to a session not yet
// subscribed to it, and an answer that is not retained is then gone.
func (c *Client) WaitSubscribed(timeout time.Duration) bool {
	select {
	case <-c.subscribed:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (c *Client) Connected() bool {
	return (*c.pahoClient).IsConnectionOpen()
}

func (c *Client) Disconnect() {
	(*c.pahoClient).Disconnect(uint(disconnectWait.Milliseconds()))
}

// clientID returns the configured client id, or for a role in sessionRoles
// derives one from the role, the hostname and the username; for any other
// role it is empty and the broker assigns one. All three matter: the
// hostname keeps two vessels (or a vessel and the cloud) apart, the role
// keeps the several gosk processes on one host apart, and the username
// keeps apart two processes of one role on one host that talk to
// different brokers - the central server runs one reader per broker. It stays
// the same across restarts, which is what lets the broker hold a session
// for it.
func clientID(c *config.MQTTConfig, role string) string {
	if c.ClientID != "" {
		return c.ClientID
	}
	if !sessionRoles[role] {
		return ""
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

	return sanitizeClientID(fmt.Sprintf("gosk-%s-%s-%s", role, host, c.Username))
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
	// a publisher that reconnects. The outbox (transfer/outbox.go) relies
	// on this for its acknowledgements.
	//
	// The broker has to be willing to hold that session: mosquitto's
	// max_queued_messages defaults to 1000, and persistence has to be on
	// for the queue to survive a broker restart.
	//
	// Only for the roles that need it (sessionRoles): the others publish
	// at QoS 0 or ask for nothing a session would keep, and connect as they
	// always did.
	if c.session {
		result.SetClientID(c.clientID)
		result.SetCleanSession(false)
		// Resume the session's subscriptions on reconnect. What this does
		// not do is carry unacknowledged publishes across a restart of the
		// process: paho's store here is in memory, so a publish in flight
		// when the process stops is gone. That is the outbox's job - the
		// rows stay in the database until acknowledged and are sent again
		// - and a receiver that never acknowledges (acknowledge: false) is
		// on its own in that respect: what it misses it does not get back.
		result.SetResumeSubs(true)
		// Drip the backlog out on reconnect instead of handing the whole
		// store to the network at once (paho's default is no limit). The
		// link that just came back is the same one that dropped.
		result.SetMaxResumePubInFlight(maxResumePubInFlight)
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

	if len(c.topics) == 0 {
		logger.GetLogger().Info(
			"Topic is empty so not subscribing",
			zap.String("URL", c.config.URLString),
		)
		return
	}

	connection := c.connection.Add(1)
	if failed := c.subscribe(pahoClient, c.topics); len(failed) > 0 {
		// Not Fatal, and not left to the next reconnect: a subscription
		// the broker refused, or one that errored, on a connection that
		// stays up would otherwise never be retried, and a caller waiting
		// on WaitSubscribed would wait for good.
		go c.retrySubscribe(pahoClient, failed, connection)
		return
	}
	c.markSubscribed(connection)
}

// subscribe subscribes to each of topics and returns those it could not.
// The broker's grant is checked as well as the error: paho reports a
// SUBACK refusal (0x80) only in the token's result, with no error.
func (c *Client) subscribe(pahoClient paho.Client, topics []string) []string {
	failed := []string{}
	for _, topic := range topics {
		token := pahoClient.Subscribe(topic, 1, nil)
		token.Wait()
		err := token.Error()
		if err == nil {
			err = granted(token, topic)
		}
		if err != nil {
			failed = append(failed, topic)
			logger.GetLogger().Warn(
				"Could not subscribe to the MQTT topic, retrying",
				zap.String("Error", err.Error()),
				zap.String("URL", c.config.URLString),
				zap.String("Topic", topic),
			)
			continue
		}
		logger.GetLogger().Info(
			"Subscribed to the MQTT topic",
			zap.String("URL", c.config.URLString),
			zap.String("Topic", topic),
		)
	}
	return failed
}

// granted reports whether the broker granted the one topic in a subscribe token
// that completed without an error. Only an explicit grant counts: paho also
// completes a token with no error and no result at all when the connection
// dropped before the broker answered and the subscription is resumed on the
// next one - which says nothing about whether it will be granted there.
func granted(token paho.Token, topic string) error {
	st, ok := token.(*paho.SubscribeToken)
	if !ok {
		return errors.New("not a subscribe token")
	}
	// One topic per Subscribe call (see subscribe), so one result - under
	// whatever key paho filed it: it strips $share/<group>/ and $queue/
	// from shared subscriptions, so looking the topic up as subscribed
	// would find nothing.
	var code byte
	results := st.Result()
	ok = len(results) == 1
	for _, c := range results {
		code = c
	}
	switch {
	case !ok:
		return errors.New("the broker did not answer the subscription")
	case code == subackRefused:
		return errors.New("the broker refused the subscription")
	case code > 2:
		return fmt.Errorf("the broker answered the subscription with %#x", code)
	}
	return nil
}

// retrySubscribe keeps subscribing to topics, backing off, until all of them
// are granted, or until connection is no longer the current one - in which
// case that connection's onConnectHandler has started over, and this worker
// would only add a second one.
func (c *Client) retrySubscribe(pahoClient paho.Client, topics []string, connection uint64) {
	wait := subscribeRetryMin
	for {
		time.Sleep(wait)
		if c.connection.Load() != connection || !pahoClient.IsConnectionOpen() {
			return
		}
		if topics = c.subscribe(pahoClient, topics); len(topics) == 0 {
			c.markSubscribed(connection)
			return
		}
		wait = min(2*wait, subscribeRetryMax)
	}
}

// markSubscribed records that every topic was granted on connection, unless
// another connection has replaced it in the meantime: its handler then
// decides.
func (c *Client) markSubscribed(connection uint64) {
	if c.connection.Load() != connection {
		return
	}
	c.subscribedOnce.Do(func() { close(c.subscribed) })
}

func connectionLostHandler(c paho.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}
}
