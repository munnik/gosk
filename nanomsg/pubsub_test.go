package nanomsg

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/message"
)

// TestPubSubMsgpRoundTrip exercises Publisher.Send/Subscriber.Receive over
// a real nanomsg socket end-to-end, the one place that reaches
// msgp.Marshaler/Unmarshaler via a runtime type assertion on *T rather
// than a generic constraint (see Message's doc comment in main.go) and
// reuses a single encode buffer across sends (see Send in pub.go) - the
// message package's own tests cover MarshalMsg/UnmarshalMsg correctness in
// isolation, but not that this wiring actually works.
func TestPubSubMsgpRoundTrip(t *testing.T) {
	url := "inproc://test-pubsub-msgp"
	pub := NewPublisher[message.Raw](url)
	sub, err := NewSubscriber[message.Raw](url, []byte{})
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
		Uuid:      uuid.New(),
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
	url := "inproc://test-pubsub-order"
	pub := NewPublisher[message.Raw](url)
	sub, err := NewSubscriber[message.Raw](url, []byte{})
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
