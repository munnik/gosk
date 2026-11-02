package transfer

import (
	"errors"
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/mqtt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeToken stands in for a paho publish token: open until done is
// closed, then complete with err.
type fakeToken struct {
	done chan struct{}
	err  error
}

func openToken() *fakeToken { return &fakeToken{done: make(chan struct{})} }

func completeToken(err error) *fakeToken {
	token := openToken()
	token.err = err
	close(token.done)
	return token
}

func (f *fakeToken) Wait() bool { <-f.done; return true }
func (f *fakeToken) WaitTimeout(d time.Duration) bool {
	select {
	case <-f.done:
		return true
	case <-time.After(d):
		return false
	}
}
func (f *fakeToken) Done() <-chan struct{} { return f.done }
func (f *fakeToken) Error() error          { return f.err }

// fakeSenderClient stands in for the MQTT client: it holds whatever
// fingerprints the test says it does, and hands out the token the test
// gave it.
type fakeSenderClient struct {
	mu      sync.Mutex
	holding map[uint64]bool
	token   paho.Token
}

func (f *fakeSenderClient) Connect()                          {}
func (f *fakeSenderClient) Disconnect()                       {}
func (f *fakeSenderClient) Connected() bool                   { return true }
func (f *fakeSenderClient) WaitSubscribed(time.Duration) bool { return true }

func (f *fakeSenderClient) Publish(string, byte, bool, []byte) paho.Token {
	if f.token == nil {
		return completeToken(nil)
	}
	return f.token
}

func (f *fakeSenderClient) Holds(fingerprint uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.holding[fingerprint]
}

func (f *fakeSenderClient) Unconfirmed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, held := range f.holding {
		if held {
			n++
		}
	}
	return n
}

func (f *fakeSenderClient) Withdraw(fingerprint uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	held := f.holding[fingerprint]
	delete(f.holding, fingerprint)
	return held
}

// confirm is the broker's confirmation of a publish.
func (f *fakeSenderClient) confirm(fingerprint uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.holding, fingerprint)
}

// inFlightSender is a Sender with just what isInFlight and its neighbours
// touch; NewSender would register its counters globally and reach for a
// database.
func inFlightSender(retryAfter time.Duration, client *fakeSenderClient) *Sender {
	return &Sender{
		config:     &config.TransferConfig{RetryAfter: retryAfter, Interval: 5 * time.Second},
		mqttClient: client,
		inFlight:   make(map[inFlightKey]inFlightEntry),
		retried:    prometheus.NewCounter(prometheus.CounterOpts{Name: "retried"}),
	}
}

var (
	testBucket  = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	testThrough = testBucket.Add(30 * time.Second)
	testEntry   = database.OutboxEntry{Origin: "vessels.1", Bucket: testBucket, Generation: 3}
	testKey     = inFlightKey{origin: testEntry.Origin, bucket: testBucket.UnixNano()}
)

func TestADeliveryTheClientStillHoldsIsInFlightHoweverOldItIs(t *testing.T) {
	payload := []byte("a delivery")
	client := &fakeSenderClient{holding: map[uint64]bool{mqtt.Fingerprint(payload): true}}
	s := inFlightSender(5*time.Minute, client)
	s.markPublished(testEntry, payload, testThrough)
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
	s.markPublished(testEntry, payload, testThrough)
	flight := s.inFlight[testKey]
	flight.sent = time.Now().Add(-time.Hour)
	s.inFlight[testKey] = flight

	// Confirmed just now, an hour after it was handed over: the far end
	// has had no time to answer yet.
	client.confirm(mqtt.Fingerprint(payload))
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

func TestAnAcknowledgementClearsTheDeliveryItReaches(t *testing.T) {
	payload := []byte("a delivery")
	client := &fakeSenderClient{holding: map[uint64]bool{mqtt.Fingerprint(payload): true}}
	s := inFlightSender(time.Hour, client)
	s.markPublished(testEntry, payload, testThrough)
	ack := Ack{Origin: testEntry.Origin, Bucket: testBucket, Generation: testEntry.Generation}

	// An earlier delivery's acknowledgement, arriving late: it reaches
	// less than this delivery does, and the client may still hold this
	// one.
	ack.Through = testThrough.Add(-time.Second)
	s.clearInFlight(ack)
	if _, kept := s.inFlight[testKey]; !kept {
		t.Fatal("a late acknowledgement of an earlier delivery cleared the entry of a later one")
	}

	// The acknowledgement of this delivery, or of a later one. That the
	// broker's confirmation reached the far end before this sender saw it
	// changes nothing.
	ack.Through = testThrough
	s.clearInFlight(ack)
	if len(s.inFlight) != 0 {
		t.Fatal("an acknowledged bucket stayed in flight")
	}
}

func TestPublishErrorReadsOnlyACompleteToken(t *testing.T) {
	if publishError(openToken()) != nil {
		t.Fatal("an open token reported an error")
	}
	if publishError(completeToken(nil)) != nil {
		t.Fatal("a token complete without an error reported one")
	}
	if publishError(completeToken(paho.ErrNotConnected)) != paho.ErrNotConnected {
		t.Fatal("a token complete with an error did not report it")
	}
}

func TestADeliveryTheClientWillNotSendIsWithdrawnAndForgotten(t *testing.T) {
	payload := []byte("a delivery")
	fingerprint := mqtt.Fingerprint(payload)

	// paho's writer did not take it: stored, to be sent on the next
	// connection, which is not what the sender wants.
	client := &fakeSenderClient{holding: map[uint64]bool{fingerprint: true}}
	s := inFlightSender(time.Hour, client)
	s.markPublished(testEntry, payload, testThrough)
	s.notSent(testEntry, fingerprint, errors.New("publish was broken by timeout"))
	if client.Holds(fingerprint) {
		t.Fatal("a delivery paho stored without sending was left in its hands")
	}
	if len(s.inFlight) != 0 {
		t.Fatal("the bucket stayed in flight")
	}

	// paho was not connected: nothing stored, nothing to withdraw.
	client = &fakeSenderClient{holding: map[uint64]bool{}}
	s = inFlightSender(time.Hour, client)
	s.markPublished(testEntry, payload, testThrough)
	s.notSent(testEntry, fingerprint, paho.ErrNotConnected)
	if len(s.inFlight) != 0 {
		t.Fatal("the bucket stayed in flight")
	}
}

func TestAwaitConfirmationReturnsOnTheBrokersConfirmation(t *testing.T) {
	payload := []byte("a delivery")
	fingerprint := mqtt.Fingerprint(payload)
	client := &fakeSenderClient{holding: map[uint64]bool{fingerprint: true}}
	s := inFlightSender(time.Hour, client)
	s.config.Interval = 10 * time.Second
	token := openToken()

	started := time.Now()
	go func() {
		time.Sleep(150 * time.Millisecond)
		client.confirm(fingerprint)
		close(token.done)
	}()
	s.awaitConfirmation(token, fingerprint)
	if waited := time.Since(started); waited < 150*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("waited %v; want until the confirmation, not until Interval", waited)
	}
}

func TestAwaitConfirmationOutlastsATokenPahoCompletedOnAReconnect(t *testing.T) {
	payload := []byte("a delivery")
	fingerprint := mqtt.Fingerprint(payload)
	client := &fakeSenderClient{holding: map[uint64]bool{fingerprint: true}}
	s := inFlightSender(time.Hour, client)
	s.config.Interval = 300 * time.Millisecond

	// The token is complete - paho re-queued the publish on a new
	// connection - but the broker has not confirmed it: still held.
	started := time.Now()
	s.awaitConfirmation(completeToken(nil), fingerprint)
	if waited := time.Since(started); waited < 300*time.Millisecond || waited > 3*time.Second {
		t.Fatalf("waited %v; want Interval, since the store still held the delivery", waited)
	}
	if !client.Holds(fingerprint) {
		t.Fatal("the fake confirmed by itself")
	}
}

func TestForgetInFlightDropsOnlyItsOwnGeneration(t *testing.T) {
	s := inFlightSender(time.Hour, &fakeSenderClient{})
	s.markPublished(testEntry, []byte("a delivery"), testThrough)
	reset := testEntry
	reset.Generation = 4
	s.forgetInFlight(reset)
	if _, kept := s.inFlight[testKey]; !kept {
		t.Fatal("forgetting a reset generation dropped the current one")
	}
	s.forgetInFlight(testEntry)
	if len(s.inFlight) != 0 {
		t.Fatal("the entry was not forgotten")
	}
}
