package connector

import (
	"testing"
	"time"

	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

// TestProcessPublishesConnectorStatus exercises process's timeout
// handling end-to-end over a real nanomsg socket: a connector whose
// stream never yields anything (the sensor never connected) must publish
// a ConnectorStatusType DisconnectedOrNoData report once timeoutDuration
// elapses, not exit the process (see process's doc comment for why - a
// connector exiting makes a merely-offline sensor indistinguishable, from
// deploy-rs/nixos-rebuild switch's point of view, from a real crash).
func TestProcessPublishesConnectorStatus(t *testing.T) {
	url := "inproc://test-process-connector-status"
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw](url, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	recvCh := make(chan *message.Raw, 8)
	go sub.Receive(recvCh)
	warmUpPubSub(t, pub, recvCh)

	stream := make(chan []byte)
	defer close(stream)
	go process(stream, "Ampero modules", "modbus", pub, 50*time.Millisecond)

	got := receiveWithTimeout(t, recvCh)
	if got.Type != message.ConnectorStatusType {
		t.Fatalf("got Type %q, want %q", got.Type, message.ConnectorStatusType)
	}
	if string(got.Value) != message.ConnectorStatusDisconnectedOrNoData {
		t.Fatalf("got Value %q, want %q", got.Value, message.ConnectorStatusDisconnectedOrNoData)
	}
	if got.Connector != "Ampero modules" {
		t.Fatalf("got Connector %q, want %q", got.Connector, "Ampero modules")
	}
}

// TestProcessRepeatsDisconnectedStatus guards the specific behaviour
// config.ConnectorConfig.Timeout's doc comment depends on: as long as the
// stream stays quiet, DisconnectedOrNoData must keep repeating every
// timeoutDuration, not fire once and go silent - gosk.nix's
// TimeoutStartSec only ever sees the *next* report after it starts
// waiting, not the first one that happened to fire.
func TestProcessRepeatsDisconnectedStatus(t *testing.T) {
	url := "inproc://test-process-connector-status-repeats"
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw](url, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	recvCh := make(chan *message.Raw, 8)
	go sub.Receive(recvCh)
	warmUpPubSub(t, pub, recvCh)

	stream := make(chan []byte)
	defer close(stream)
	go process(stream, "Ampero modules", "modbus", pub, 50*time.Millisecond)

	for i := 0; i < 3; i++ {
		got := receiveWithTimeout(t, recvCh)
		if got.Type != message.ConnectorStatusType || string(got.Value) != message.ConnectorStatusDisconnectedOrNoData {
			t.Fatalf("report %d = %+v, want a DisconnectedOrNoData status report", i, got)
		}
	}
}

// TestProcessPublishesConnectedThenData mirrors real data arriving: it
// exercises the other half of process's status tracking, that a value
// arriving after (or without) a prior timeout is preceded by a
// ConnectedAndData report before the real message.
func TestProcessPublishesConnectedThenData(t *testing.T) {
	url := "inproc://test-process-connector-status-connected"
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw](url, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	recvCh := make(chan *message.Raw, 8)
	go sub.Receive(recvCh)
	warmUpPubSub(t, pub, recvCh)

	stream := make(chan []byte, 1)
	defer close(stream)
	// timeoutDuration comfortably longer than this test takes to run, so
	// only the real value below - not a timeout - produces any message.
	go process(stream, "Ampero modules", "modbus", pub, time.Minute)
	stream <- []byte{0x01, 0x02, 0x03}

	first := receiveWithTimeout(t, recvCh)
	if first.Type != message.ConnectorStatusType || string(first.Value) != message.ConnectorStatusConnectedAndData {
		t.Fatalf("first message = %+v, want a ConnectedAndData status report", first)
	}

	second := receiveWithTimeout(t, recvCh)
	if second.Type != "modbus" {
		t.Fatalf("second message Type = %q, want %q", second.Type, "modbus")
	}
	if string(second.Value) != "\x01\x02\x03" {
		t.Fatalf("second message Value = %v, want the real value", second.Value)
	}
}

// warmUpPubSub blocks until a throwaway message sent on pub is actually
// received on recvCh, proving the subscriber's pipe is fully connected
// before the real test traffic starts. nanomsg pub/sub is lossy by
// design - mangos drops a message if a pipe isn't ready yet, rather than
// buffering it (see nanomsg/pubsub_test.go's TestPubSubPreservesOrder),
// and Publisher.Send draining its input channel only means the attempt
// was made, not that mangos actually delivered it - so a fixed sleep
// before sending is a guess, not a guarantee, and was observed flaking in
// exactly this package (TestProcessPublishesConnectedThenData missing its
// first message under a 100ms sleep). Retries its own send, not just the
// receive, since the drop can happen on either the very first attempt.
func warmUpPubSub(t *testing.T, pub *nanomsg.Publisher[message.Raw], recvCh chan *message.Raw) {
	t.Helper()

	for attempt := 0; attempt < 20; attempt++ {
		warmUp := make(chan *message.Raw, 1)
		warmUp <- message.NewRaw().WithConnector("warmup").WithType("warmup").WithValue(nil)
		close(warmUp)
		pub.Send(warmUp) // blocks until the message is sent, or dropped - either way, drained

		select {
		case got := <-recvCh:
			if got.Connector == "warmup" {
				return
			}
			// Sent while a previous run's subscriber was still catching
			// up, or this func was called more than once against the
			// same recvCh - not this warm-up's own message, so keep
			// waiting for it. Nothing in this package does either, but
			// failing loudly on unexpected input beats silently
			// swallowing a real message.
			t.Fatalf("got an unexpected message while warming up: %+v", got)
		case <-time.After(10 * time.Millisecond):
			// dropped - the pipe likely still isn't ready - try again
		}
	}
	t.Fatal("warmUpPubSub: pub/sub pipe never became ready")
}

func receiveWithTimeout(t *testing.T, ch <-chan *message.Raw) *message.Raw {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a message")
		return nil
	}
}
