package transfer

import (
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/mqtt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeSenderClient stands in for the MQTT client: it holds whatever
// fingerprints the test says it does.
type fakeSenderClient struct {
	holding map[uint64]bool
}

func (f *fakeSenderClient) Connect()                           {}
func (f *fakeSenderClient) Disconnect()                        {}
func (f *fakeSenderClient) Connected() bool                    { return true }
func (f *fakeSenderClient) WaitSubscribed(time.Duration) bool  { return true }
func (f *fakeSenderClient) Publish(string, byte, bool, []byte) {}
func (f *fakeSenderClient) Holds(fingerprint uint64) bool      { return f.holding[fingerprint] }

// inFlightSender is a Sender with just what isInFlight and its neighbours
// touch; NewSender would register its counters globally and reach for a
// database.
func inFlightSender(retryAfter time.Duration, client *fakeSenderClient) *Sender {
	return &Sender{
		config:     &config.TransferConfig{RetryAfter: retryAfter},
		mqttClient: client,
		inFlight:   make(map[inFlightKey]inFlightEntry),
		retried:    prometheus.NewCounter(prometheus.CounterOpts{Name: "retried"}),
	}
}

var (
	testBucket = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	testEntry  = database.OutboxEntry{Origin: "vessels.1", Bucket: testBucket, Generation: 3}
	testKey    = inFlightKey{origin: testEntry.Origin, bucket: testBucket.UnixNano()}
)

func TestADeliveryTheClientStillHoldsIsInFlightHoweverOldItIs(t *testing.T) {
	payload := []byte("a delivery")
	client := &fakeSenderClient{holding: map[uint64]bool{mqtt.Fingerprint(payload): true}}
	s := inFlightSender(5*time.Minute, client)
	s.markPublished(testEntry, payload)
	flight := s.inFlight[testKey]
	flight.sent = time.Now().Add(-24 * time.Hour)
	s.inFlight[testKey] = flight

	if !s.isInFlight(testEntry) {
		t.Fatal("a bucket whose delivery the client still holds was sent again")
	}
	if got := testutil.ToFloat64(s.retried); got != 0 {
		t.Fatalf("retried = %v, want 0", got)
	}
}

func TestRetryAfterCountsFromTheBrokersConfirmation(t *testing.T) {
	payload := []byte("a delivery")
	client := &fakeSenderClient{holding: map[uint64]bool{mqtt.Fingerprint(payload): true}}
	s := inFlightSender(5*time.Minute, client)
	s.markPublished(testEntry, payload)
	flight := s.inFlight[testKey]
	flight.sent = time.Now().Add(-time.Hour)
	s.inFlight[testKey] = flight

	// Confirmed just now, an hour after it was handed over: the far end
	// has had no time to answer yet.
	client.holding = map[uint64]bool{}
	if !s.isInFlight(testEntry) {
		t.Fatal("sent again the moment the broker confirmed it, before the far end could answer")
	}
	if s.inFlight[testKey].confirmed.IsZero() {
		t.Fatal("the confirmation was not recorded")
	}

	// RetryAfter after the confirmation, with no acknowledgement: again.
	flight = s.inFlight[testKey]
	flight.confirmed = time.Now().Add(-6 * time.Minute)
	s.inFlight[testKey] = flight
	if s.isInFlight(testEntry) {
		t.Fatal("not sent again RetryAfter after the broker confirmed it")
	}
	if _, kept := s.inFlight[testKey]; kept {
		t.Fatal("the entry lingered")
	}
	if got := testutil.ToFloat64(s.retried); got != 1 {
		t.Fatalf("retried = %v, want 1", got)
	}
}

func TestABucketHeldBackWithoutADeliveryWaitsRetryAfterFromThen(t *testing.T) {
	s := inFlightSender(5*time.Minute, &fakeSenderClient{})
	s.markInFlight(testEntry)
	if !s.isInFlight(testEntry) {
		t.Fatal("not held back")
	}
	flight := s.inFlight[testKey]
	flight.sent = time.Now().Add(-6 * time.Minute)
	s.inFlight[testKey] = flight
	if s.isInFlight(testEntry) {
		t.Fatal("still held back after RetryAfter")
	}
}

func TestInFlightForgetsAResetGeneration(t *testing.T) {
	s := inFlightSender(time.Hour, &fakeSenderClient{})
	s.markInFlight(testEntry)
	reset := testEntry
	reset.Generation = 4
	if s.isInFlight(reset) {
		t.Fatal("a bucket reset to a new generation was still held back for the old one")
	}
	if len(s.inFlight) != 0 {
		t.Fatal("the old generation's entry lingered")
	}
}

func TestAnAcknowledgementClearsTheEntry(t *testing.T) {
	payload := []byte("a delivery")
	client := &fakeSenderClient{holding: map[uint64]bool{mqtt.Fingerprint(payload): true}}
	s := inFlightSender(time.Hour, client)
	s.markPublished(testEntry, payload)
	// The broker's confirmation reached the far end before this sender
	// saw it; the acknowledgement settles the bucket all the same.
	s.clearInFlight(Ack{Origin: testEntry.Origin, Bucket: testBucket, Generation: testEntry.Generation})
	if len(s.inFlight) != 0 {
		t.Fatal("an acknowledged bucket stayed in flight")
	}
	if s.countUnconfirmed() != 0 {
		t.Fatal("an acknowledged bucket was counted as unconfirmed")
	}
}

func TestCountUnconfirmedCountsWhatTheClientHolds(t *testing.T) {
	held, confirmed := []byte("held"), []byte("confirmed")
	client := &fakeSenderClient{holding: map[uint64]bool{mqtt.Fingerprint(held): true}}
	s := inFlightSender(time.Hour, client)
	s.markPublished(testEntry, held)
	other := testEntry
	other.Bucket = testBucket.Add(time.Minute)
	s.markPublished(other, confirmed)
	s.markInFlight(database.OutboxEntry{Origin: "vessels.1", Bucket: testBucket.Add(2 * time.Minute), Generation: 9})
	if n := s.countUnconfirmed(); n != 1 {
		t.Fatalf("unconfirmed = %d, want 1", n)
	}
}
