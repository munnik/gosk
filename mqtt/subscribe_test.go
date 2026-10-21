package mqtt

import (
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/eclipse/paho.mqtt.golang/packets"
	"github.com/munnik/gosk/config"
)

// fakeBroker speaks just enough MQTT over an in-memory pipe to accept a
// connection and answer subscriptions, refusing the topic it is told to
// refuse the first refusals times it is asked for.
type fakeBroker struct {
	mu       sync.Mutex
	refuse   string
	refusals int
	asked    map[string]int
	granted  map[string]int
	// drop, when set, is a topic whose first subscription makes the
	// broker close the connection instead of answering.
	drop    string
	dropped bool
}

func (b *fakeBroker) open(*url.URL, paho.ClientOptions) (net.Conn, error) {
	server, client := net.Pipe()
	go b.serve(server)
	return client, nil
}

func (b *fakeBroker) serve(conn net.Conn) {
	defer conn.Close()
	for {
		packet, err := packets.ReadPacket(conn)
		if err != nil {
			return
		}
		switch p := packet.(type) {
		case *packets.ConnectPacket:
			ack := packets.NewControlPacket(packets.Connack).(*packets.ConnackPacket)
			if ack.Write(conn) != nil {
				return
			}
		case *packets.SubscribePacket:
			ack := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
			ack.MessageID = p.MessageID
			b.mu.Lock()
			drop := false
			for _, topic := range p.Topics {
				b.asked[topic]++
				if topic == b.drop && !b.dropped {
					b.dropped, drop = true, true
				}
				if topic == b.refuse && b.asked[topic] <= b.refusals {
					ack.ReturnCodes = append(ack.ReturnCodes, subackRefused)
				} else {
					ack.ReturnCodes = append(ack.ReturnCodes, 1)
					b.granted[topic]++
				}
			}
			b.mu.Unlock()
			if drop {
				return
			}
			if ack.Write(conn) != nil {
				return
			}
		case *packets.PingreqPacket:
			if packets.NewControlPacket(packets.Pingresp).Write(conn) != nil {
				return
			}
		case *packets.DisconnectPacket:
			return
		}
	}
}

func (b *fakeBroker) timesGranted(topic string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.drop == topic && b.dropped && b.granted[topic] > 0 {
		// The dropped connection's grant was never sent.
		return b.granted[topic] - 1
	}
	return b.granted[topic]
}

func (b *fakeBroker) timesAsked(topic string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.asked[topic]
}

func clientOn(t *testing.T, broker *fakeBroker, topics ...string) *Client {
	t.Helper()
	c := New(&config.MQTTConfig{URLString: "tcp://fake.invalid:1883", Username: "test", Password: "test"}, "test", nil, topics[0], topics[1:]...)
	options := c.createClientOptions()
	options.SetCustomOpenConnectionFn(broker.open)
	pahoClient := paho.NewClient(options)
	c.pahoClient = &pahoClient
	c.Connect()
	t.Cleanup(c.Disconnect)
	return c
}

// TestARefusedSubscriptionIsNotReadyAndIsRetried: paho reports a SUBACK
// refusal only in the token's result, with no error. A client that took it
// for a subscription would let the outbox sender ask for its epoch with
// nothing listening for the answer; one that never retried it would leave
// the sender waiting for good on a connection that stays up.
func TestARefusedSubscriptionIsNotReadyAndIsRetried(t *testing.T) {
	broker := &fakeBroker{refuse: "reply/topic", refusals: 1, asked: map[string]int{}, granted: map[string]int{}}
	c := clientOn(t, broker, "ack/topic", "reply/topic")

	if c.WaitSubscribed(500 * time.Millisecond) {
		t.Fatal("reported subscribed while the broker had refused one of the topics")
	}
	if !c.WaitSubscribed(5 * time.Second) {
		t.Fatalf("never subscribed after the broker stopped refusing; asked %d times", broker.timesAsked("reply/topic"))
	}
	if n := broker.timesAsked("reply/topic"); n < 2 {
		t.Fatalf("the refused topic was asked for %d times, expected a retry", n)
	}
	if n := broker.timesAsked("ack/topic"); n != 1 {
		t.Fatalf("the granted topic was asked for %d times, expected once", n)
	}
}

func TestASubscriptionTheBrokerKeepsRefusingIsNeverReady(t *testing.T) {
	broker := &fakeBroker{refuse: "reply/topic", refusals: 1 << 30, asked: map[string]int{}, granted: map[string]int{}}
	c := clientOn(t, broker, "ack/topic", "reply/topic")

	if c.WaitSubscribed(2500 * time.Millisecond) {
		t.Fatal("reported subscribed while the broker refused the topic every time")
	}
	if n := broker.timesAsked("reply/topic"); n < 2 {
		t.Fatalf("the refused topic was asked for %d times, expected retries", n)
	}
}

func TestEveryTopicGrantedIsReady(t *testing.T) {
	broker := &fakeBroker{asked: map[string]int{}, granted: map[string]int{}}
	c := clientOn(t, broker, "ack/topic", "reply/topic")

	if !c.WaitSubscribed(5 * time.Second) {
		t.Fatal("never reported subscribed although every topic was granted")
	}
}

// TestAConnectionLostBeforeTheSubackIsNotReady: paho resumes a subscription
// whose SUBACK never came on the next connection, and completes the
// original token with no error and no result. Taking that for a grant made
// the client ready with the reply topic never granted on any connection.
func TestAConnectionLostBeforeTheSubackIsNotReady(t *testing.T) {
	broker := &fakeBroker{drop: "reply/topic", asked: map[string]int{}, granted: map[string]int{}}
	c := clientOn(t, broker, "ack/topic", "reply/topic")

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if c.WaitSubscribed(50 * time.Millisecond) {
			if broker.timesGranted("reply/topic") == 0 {
				t.Fatal("reported subscribed with the reply topic never granted")
			}
			return
		}
	}
	t.Fatalf("never subscribed after reconnecting; reply topic asked %d times, granted %d", broker.timesAsked("reply/topic"), broker.timesGranted("reply/topic"))
}

// TestARetryWorkerOfAReplacedConnectionStops: a reconnect starts a worker of
// its own, so one left from the connection before must not keep going.
func TestARetryWorkerOfAReplacedConnectionStops(t *testing.T) {
	broker := &fakeBroker{asked: map[string]int{}, granted: map[string]int{}}
	c := clientOn(t, broker, "ack/topic")
	if !c.WaitSubscribed(5 * time.Second) {
		t.Fatal("not subscribed")
	}

	done := make(chan struct{})
	go func() {
		c.retrySubscribe(*c.pahoClient, []string{"other/topic"}, c.connection.Load()+1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a worker of a connection that is not the current one kept going")
	}
	if n := broker.timesAsked("other/topic"); n != 0 {
		t.Fatalf("a stale worker subscribed %d times", n)
	}
}

// TestASharedSubscriptionIsReady: paho files the grant of
// "$share/<group>/<topic>" under "<topic>", so the grant has to be read
// without looking the topic up as it was subscribed.
func TestASharedSubscriptionIsReady(t *testing.T) {
	broker := &fakeBroker{asked: map[string]int{}, granted: map[string]int{}}
	c := clientOn(t, broker, "$share/workers/reply/topic", "$queue/other/topic")

	if !c.WaitSubscribed(5 * time.Second) {
		t.Fatalf("a granted shared subscription was not ready; asked %d and %d times", broker.timesAsked("$share/workers/reply/topic"), broker.timesAsked("$queue/other/topic"))
	}
	if n := broker.timesAsked("$share/workers/reply/topic"); n != 1 {
		t.Fatalf("the shared subscription was asked for %d times, expected once", n)
	}
}
