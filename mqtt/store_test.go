package mqtt

import (
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
)

func TestTrackedStoreHoldsAnOutboundPublishUntilItIsDeleted(t *testing.T) {
	store := newTrackedStore()
	store.Open()

	payload := []byte("a delivery")
	publish := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	publish.Qos = 1
	publish.MessageID = 7
	publish.Payload = payload
	if store.holds(Fingerprint(payload)) {
		t.Fatal("held before anything was put")
	}

	store.Put("o.7", publish)
	if !store.holds(Fingerprint(payload)) || store.unconfirmed() != 1 {
		t.Fatal("not held after the publish was put")
	}
	if store.Get("o.7") != publish || len(store.All()) != 1 {
		t.Fatal("the store does not keep what paho gave it")
	}
	if store.holds(Fingerprint([]byte("another delivery"))) {
		t.Fatal("held a payload that was never put")
	}

	// The same publish received, as paho would key it: not an outbound
	// publish, so not held under its fingerprint twice.
	store.Put("i.7", publish)
	store.Del("i.7")
	if !store.holds(Fingerprint(payload)) {
		t.Fatal("deleting the inbound copy forgot the outbound one")
	}

	// The broker's PUBACK.
	store.Del("o.7")
	if store.holds(Fingerprint(payload)) || store.unconfirmed() != 0 || len(store.All()) != 0 {
		t.Fatal("still held after the publish was deleted")
	}
	// Deleting it again, or something never put, is nothing.
	store.Del("o.7")
	store.Del("o.8")
}

func TestTrackedStoreResetForgetsEverything(t *testing.T) {
	store := newTrackedStore()
	publish := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	publish.Qos = 1
	publish.Payload = []byte("a delivery")
	store.Put("o.1", publish)
	store.Put("o.2", publish)
	store.Reset()
	if store.holds(Fingerprint(publish.Payload)) || len(store.All()) != 0 {
		t.Fatal("Reset left something behind")
	}
}

// publishedTimes waits until the broker has seen topic published n times,
// or gives up after timeout, and reports the count it saw last.
func publishedTimes(broker *fakeBroker, topic string, n int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		seen := broker.timesPublished(topic)
		if seen >= n || time.Now().After(deadline) {
			return seen
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestASenderClientHoldsAPublishUntilTheBrokerConfirmsIt: the outbox
// sender must not send a bucket again while paho still holds its last
// delivery, so Holds has to stay true across the loss of a connection -
// paho re-sends the publish on the next one - and turn false only on the
// broker's confirmation. The broker here reads the publish without
// confirming it, the link then drops, and the broker confirms the copy
// paho sends on the new connection.
func TestASenderClientHoldsAPublishUntilTheBrokerConfirmsIt(t *testing.T) {
	broker := &fakeBroker{asked: map[string]int{}, granted: map[string]int{}, withholdConfirmations: true}
	c := clientOnAs(t, broker, "transferSend", "ack/topic")
	if !c.WaitSubscribed(5 * time.Second) {
		t.Fatal("never subscribed")
	}

	payload := []byte("a delivery")
	c.Publish("data/topic", 1, false, payload)
	if n := publishedTimes(broker, "data/topic", 1, 5*time.Second); n != 1 {
		t.Fatalf("the broker saw the publish %d times, want 1", n)
	}
	if !c.Holds(Fingerprint(payload)) {
		t.Fatal("not held while the broker has not confirmed the publish")
	}

	// The link drops. paho reconnects and re-sends what the broker did
	// not confirm; the publish is held throughout, and the broker now
	// confirms it.
	broker.confirmPublishes(true)
	broker.dropConnection()
	if n := publishedTimes(broker, "data/topic", 2, 10*time.Second); n != 2 {
		t.Fatalf("the broker saw the publish %d times after the reconnect, want 2", n)
	}
	deadline := time.Now().Add(5 * time.Second)
	for c.Holds(Fingerprint(payload)) {
		if time.Now().After(deadline) {
			t.Fatal("still held after the broker confirmed the re-sent publish")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAClientOfAnotherRoleHoldsNothing(t *testing.T) {
	broker := &fakeBroker{asked: map[string]int{}, granted: map[string]int{}}
	c := clientOnAs(t, broker, "transferReceive", "data/topic")
	if !c.WaitSubscribed(5 * time.Second) {
		t.Fatal("never subscribed")
	}
	payload := []byte("an acknowledgement")
	c.Publish("ack/topic", 1, false, payload)
	if c.Holds(Fingerprint(payload)) {
		t.Fatal("a role outside trackedRoles reported a publish held")
	}
}
