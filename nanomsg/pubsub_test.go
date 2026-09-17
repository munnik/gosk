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
// reuses a pooled buffer across sends (see msgBufferPool in pub.go) - the
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
