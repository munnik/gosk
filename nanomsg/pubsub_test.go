package nanomsg

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/message"
)

var inprocSeq atomic.Uint64

// uniqueInprocURL returns a nanomsg url that no other test, and no
// earlier run of this one, has listened on.
//
// mangos keeps its inproc listeners in a process-wide table and
// NewPublisher never takes one back out of it, so a url spelled out as a
// constant only works once per test binary. Run the package twice in one
// process - `go test -count=2`, or the same package listed twice on one
// command line - and the second NewPublisher fails with "address in
// use", which is Fatal: it takes the whole binary down rather than
// failing the one test, so every later test in the package disappears
// with it.
func uniqueInprocURL(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("inproc://%s-%d", t.Name(), inprocSeq.Add(1))
}

// TestPubSubMsgpRoundTrip exercises Publisher.Send/Subscriber.Receive over
// a real nanomsg socket end-to-end, the one place that reaches
// msgp.Marshaler/Unmarshaler via a runtime type assertion on *T rather
// than a generic constraint (see Message's doc comment in main.go) and
// reuses a single encode buffer across sends (see Send in pub.go) - the
// message package's own tests cover MarshalMsg/UnmarshalMsg correctness in
// isolation, but not that this wiring actually works.
func TestPubSubMsgpRoundTrip(t *testing.T) {
	url := uniqueInprocURL(t)
	pub := NewPublisher[message.Raw](url)
	sub, err := NewSubscriber[message.Raw]([]string{url}, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	sendCh := make(chan *message.Raw, 4)
	recvCh := make(chan *message.Raw, 4)
	go pub.Send(sendCh)
	go sub.Receive(recvCh)

	time.Sleep(100 * time.Millisecond)

	original := message.Raw{
		Connector: "testConnector",
		Timestamp: time.Now(),
		Type:      "manner_ethernet",
		Uuid:      uuid.Must(uuid.NewV7()),
		Value:     []byte{0x01, 0x02, 0x03},
	}
	sendCh <- &original

	select {
	case got := <-recvCh:
		if !got.Equals(original) {
			t.Fatalf("round-tripped value differs:\n got:  %+v\n want: %+v", got, original)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

// TestPubSubPreservesOrder guards Publisher.Send's ordering. Marshal-and-send
// used to run in a goroutine per message, which let the scheduler decide
// which message reached socket.Send first and reordered the stream (see
// Send's doc comment in pub.go for why that isn't just cosmetic - a mapper
// keeping "the latest value" state across messages, e.g. AggregateMapper or
// ModbusMapper, has no way to tell a reordered message from a genuinely new
// one). Only the relative order of what arrives is asserted: nanomsg pub/sub
// is lossy by design - mangos drops when a pipe's send queue is full - so
// the test must not depend on every message being delivered.
func TestPubSubPreservesOrder(t *testing.T) {
	url := uniqueInprocURL(t)
	pub := NewPublisher[message.Raw](url)
	sub, err := NewSubscriber[message.Raw]([]string{url}, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	const n = 500
	sendCh := make(chan *message.Raw, n)
	recvCh := make(chan *message.Raw, n)
	go sub.Receive(recvCh)

	// warm up the pipe with a throwaway message before the real run, so a
	// slow first connection doesn't itself masquerade as reordering - see
	// connector.warmUpPubSub's doc comment for why a fixed sleep is a
	// guess, not a guarantee, for this.
	for {
		warmUp := make(chan *message.Raw, 1)
		warmUp <- message.NewRaw().WithConnector("warmup").WithType("warmup").WithValue(nil)
		close(warmUp)
		pub.Send(warmUp)
		select {
		case got := <-recvCh:
			if got.Connector == "warmup" {
				goto warmedUp
			}
			t.Fatalf("got an unexpected message while warming up: %+v", got)
		case <-time.After(10 * time.Millisecond):
		}
	}
warmedUp:

	go pub.Send(sendCh)
	for i := 0; i < n; i++ {
		sendCh <- message.NewRaw().WithConnector("testConnector").WithType("manner_ethernet").WithValue([]byte{byte(i), byte(i >> 8)})
	}
	close(sendCh)

	last := -1
	received := 0
	for i := 0; i < n; i++ {
		select {
		case got := <-recvCh:
			seq := int(got.Value[0]) | int(got.Value[1])<<8
			if seq <= last {
				t.Fatalf("message %d arrived out of order: sequence %d after %d", i, seq, last)
			}
			last = seq
			received++
		case <-time.After(2 * time.Second):
			i = n // stop waiting - a lossy transport dropping the tail isn't itself a failure
		}
	}
	if received == 0 {
		t.Fatal("received no messages at all")
	}
}

// TestSubscriberReceivesFromEveryPublisher is the behaviour that replaced
// the proxy process: one subscriber dialling several publishers gets every
// publisher's messages on the same stream. Before this, a subscriber held a
// single url and a separate `gosk proxy` process existed for the sole
// purpose of fanning several publishers into one - so a processor could
// never name more than one upstream itself, and repeating --subscribeURL
// silently kept only the last one.
func TestSubscriberReceivesFromEveryPublisher(t *testing.T) {
	urls := []string{
		uniqueInprocURL(t),
		uniqueInprocURL(t),
		uniqueInprocURL(t),
	}
	publishers := make([]chan *message.Raw, len(urls))
	for i, url := range urls {
		pub := NewPublisher[message.Raw](url)
		publishers[i] = make(chan *message.Raw, 4)
		go pub.Send(publishers[i])
	}

	sub, err := NewSubscriber[message.Raw](urls, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	recvCh := make(chan *message.Raw, 16)
	go sub.Receive(recvCh)

	time.Sleep(100 * time.Millisecond)

	for i, ch := range publishers {
		ch <- &message.Raw{
			Connector: fmt.Sprintf("connector-%d", i),
			Timestamp: time.Now(),
			Type:      "manner_ethernet",
			Uuid:      uuid.Must(uuid.NewV7()),
			Value:     []byte{byte(i)},
		}
	}

	// every publisher must be heard from, in whatever order they arrive
	seen := make(map[string]bool, len(urls))
	deadline := time.After(5 * time.Second)
	for len(seen) < len(urls) {
		select {
		case got := <-recvCh:
			seen[got.Connector] = true
		case <-deadline:
			t.Fatalf("timed out, only received from %v of %d publishers: %v", len(seen), len(urls), seen)
		}
	}
}

// TestNewSubscriberWithoutUrls rejects a subscription to nothing outright,
// rather than starting a processor that can never receive anything.
func TestNewSubscriberWithoutUrls(t *testing.T) {
	if _, err := NewSubscriber[message.Raw](nil, []byte{}); err == nil {
		t.Fatal("expected an error when subscribing to no url at all")
	}
}

// TestNewSubscriberDoesNotWaitForThePublisher covers the other half of the
// change: a publisher that is not listening yet must not stop the
// subscriber from being created, or one dead upstream would keep a
// processor from ever reading the upstreams next to it.
func TestNewSubscriberDoesNotWaitForThePublisher(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		_, err := NewSubscriber[message.Raw]([]string{"tcp://127.0.0.1:1"}, []byte{})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("NewSubscriber: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("NewSubscriber blocked on a publisher that is not listening")
	}
}
