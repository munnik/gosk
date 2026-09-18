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
// a ConnectorStatusType Disconnected report once timeoutDuration elapses,
// not exit the process (see process's doc comment for why - a connector
// exiting makes a merely-offline sensor indistinguishable, from
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
	time.Sleep(100 * time.Millisecond) // let the inproc connection establish, see pubsub_test.go

	stream := make(chan []byte)
	defer close(stream)
	go process(stream, "Ampero modules", "modbus", pub, 50*time.Millisecond)

	select {
	case got := <-recvCh:
		if got.Type != message.ConnectorStatusType {
			t.Fatalf("got Type %q, want %q", got.Type, message.ConnectorStatusType)
		}
		if string(got.Value) != message.ConnectorStatusDisconnected {
			t.Fatalf("got Value %q, want %q", got.Value, message.ConnectorStatusDisconnected)
		}
		if got.Connector != "Ampero modules" {
			t.Fatalf("got Connector %q, want %q", got.Connector, "Ampero modules")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a disconnected status report")
	}
}

// TestProcessPublishesConnectedThenData mirrors real data arriving: it
// exercises the other half of process's status tracking, that a value
// arriving after (or without) a prior timeout is preceded by a Connected
// report before the real message.
func TestProcessPublishesConnectedThenData(t *testing.T) {
	url := "inproc://test-process-connector-status-connected"
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw](url, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	recvCh := make(chan *message.Raw, 8)
	go sub.Receive(recvCh)
	time.Sleep(100 * time.Millisecond)

	stream := make(chan []byte, 1)
	defer close(stream)
	// timeoutDuration comfortably longer than this test takes to run, so
	// only the real value below - not a timeout - produces any message.
	go process(stream, "Ampero modules", "modbus", pub, time.Minute)
	stream <- []byte{0x01, 0x02, 0x03}

	first := receiveWithTimeout(t, recvCh)
	if first.Type != message.ConnectorStatusType || string(first.Value) != message.ConnectorStatusConnected {
		t.Fatalf("first message = %+v, want a Connected status report", first)
	}

	second := receiveWithTimeout(t, recvCh)
	if second.Type != "modbus" {
		t.Fatalf("second message Type = %q, want %q", second.Type, "modbus")
	}
	if string(second.Value) != "\x01\x02\x03" {
		t.Fatalf("second message Value = %v, want the real value", second.Value)
	}
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
