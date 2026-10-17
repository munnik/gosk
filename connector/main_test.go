package connector

import (
	"fmt"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

var inprocSeq atomic.Uint64

// uniqueInprocURL returns a nanomsg url nothing in this process has
// listened on yet. A url spelled out as a constant only works once per
// test binary; see nanomsg's own uniqueInprocURL for why.
func uniqueInprocURL(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("inproc://%s-%d", t.Name(), inprocSeq.Add(1))
}

// TestProcessPublishesConnectorStatus exercises process's timeout
// handling end-to-end over a real nanomsg socket: a connector whose
// stream never yields anything (the sensor never connected) must publish
// a ConnectorStatusType DisconnectedOrNoData report once timeoutDuration
// elapses, not exit the process (see process's doc comment for why - a
// connector exiting makes a merely-offline sensor indistinguishable, from
// deploy-rs/nixos-rebuild switch's point of view, from a real crash).
func TestProcessPublishesConnectorStatus(t *testing.T) {
	url := uniqueInprocURL(t)
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw]([]string{url}, []byte{})
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
	url := uniqueInprocURL(t)
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw]([]string{url}, []byte{})
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
	url := uniqueInprocURL(t)
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw]([]string{url}, []byte{})
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

// TestProcessReconnectsAfterATimeout covers the transition the timeout
// used to be able to corrupt: a connector that has been reported
// DisconnectedOrNoData must report ConnectedAndData again as soon as data
// arrives. process tracked that with a bool shared between this loop and
// an AfterFunc callback on the timer's goroutine, and a timeout landing
// between the loop's read of it and its write left the loop believing it
// had already announced a connection it never announced - so the
// connector stayed reported as offline, every timeout, for as long as the
// process lived, while data flowed the whole time. See process.
func TestProcessReconnectsAfterATimeout(t *testing.T) {
	url := uniqueInprocURL(t)
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw]([]string{url}, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	recvCh := make(chan *message.Raw, 8)
	go sub.Receive(recvCh)
	warmUpPubSub(t, pub, recvCh)

	stream := make(chan []byte, 1)
	defer close(stream)
	go process(stream, "Ampero modules", "modbus", pub, 50*time.Millisecond)

	first := receiveWithTimeout(t, recvCh)
	if first.Type != message.ConnectorStatusType || string(first.Value) != message.ConnectorStatusDisconnectedOrNoData {
		t.Fatalf("first message = %+v, want a DisconnectedOrNoData status report", first)
	}

	stream <- []byte{0x01, 0x02, 0x03}

	// The stream stays quiet until this point, so more disconnected
	// reports may be in flight ahead of the value; the connector coming
	// back is what has to follow them.
	for {
		got := receiveWithTimeout(t, recvCh)
		if got.Type == message.ConnectorStatusType && string(got.Value) == message.ConnectorStatusDisconnectedOrNoData {
			continue
		}
		if got.Type != message.ConnectorStatusType || string(got.Value) != message.ConnectorStatusConnectedAndData {
			t.Fatalf("got %+v, want a ConnectedAndData status report once data arrived", got)
		}
		break
	}

	data := receiveWithTimeout(t, recvCh)
	if data.Type != "modbus" || string(data.Value) != "\x01\x02\x03" {
		t.Fatalf("got %+v, want the real value after the ConnectedAndData report", data)
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

// TestProcessReportsReadyBeforeAnySensorData covers the readiness half of
// what makes an absent sensor a notification rather than a failed deploy:
// process tells systemd the unit has started as soon as it is running,
// without waiting for the sensor, for a status report, or for a timeout.
//
// Readiness used to arrive implicitly, on the publisher's first successful
// send (see nanomsg/pub.go's send), which for a connector whose sensor is
// absent is a whole Timeout after start - so Timeout had to be kept under
// gosk.nix's TimeoutStartSec (40s) or the unit never started at all, and
// the MQTT connector, which waited up to ten seconds for its broker before
// process even began, sat right on that boundary. Nothing about how long a
// connector waits before calling a sensor missing should be able to decide
// whether the unit starts.
func TestProcessReportsReadyBeforeAnySensorData(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "notify.sock")
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		t.Fatalf("could not listen on a test unixgram socket: %v", err)
	}
	defer listener.Close()
	t.Setenv("NOTIFY_SOCKET", socketPath)

	stream := make(chan []byte)
	defer close(stream)
	// An hour is far longer than this test waits below, so a READY
	// datagram arriving at all can only have come from process starting -
	// not from a timeout, a status report or any data.
	go process(stream, "Ampero modules", "modbus", nanomsg.NewPublisher[message.Raw](uniqueInprocURL(t)), time.Hour)

	listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := listener.Read(buf)
	if err != nil {
		t.Fatalf("expected a READY datagram from a connector with no sensor, got error: %v", err)
	}
	if got := string(buf[:n]); got != "READY=1" {
		t.Fatalf("got %q, want %q", got, "READY=1")
	}
}

// TestProcessRepeatsConnectedStatus guards the recovery direction of the
// status reports. ConnectedAndData is only published on the
// disconnected->connected transition, which for a healthy sensor happens
// once in the first moments of the process; nanomsg pub/sub is lossy and
// has no replay for a late subscriber (see mapper/main.go's process), so a
// mapper that had raised the offline notification and then restarted -
// routine for a mapper unit - would never hear that the connector is fine
// again, and would leave that alarm up indefinitely on a healthy sensor.
// So the healthy state has to be re-announced on the same interval the
// unhealthy one already was, even while data flows continuously and the
// timeout therefore never fires.
func TestProcessRepeatsConnectedStatus(t *testing.T) {
	url := uniqueInprocURL(t)
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw]([]string{url}, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	recvCh := make(chan *message.Raw, 64)
	go sub.Receive(recvCh)
	warmUpPubSub(t, pub, recvCh)

	// stream is deliberately never closed: the feeder below owns it, and
	// stopping the feeder and closing the channel cannot be ordered
	// against each other without racing its in-flight send.
	stream := make(chan []byte)
	go process(stream, "Ampero modules", "modbus", pub, 50*time.Millisecond)

	// Keep data arriving far faster than the timeout, so the timeout is
	// reset before it ever fires and every status report below can only
	// have come from the heartbeat.
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				select {
				case stream <- []byte{0x01, 0x02, 0x03}:
				case <-stop:
					return
				}
			}
		}
	}()

	connectedReports := 0
	deadline := time.After(5 * time.Second)
	for connectedReports < 3 {
		select {
		case got := <-recvCh:
			if got.Type != message.ConnectorStatusType {
				continue // a real value
			}
			if string(got.Value) != message.ConnectorStatusConnectedAndData {
				t.Fatalf("got status %q while data was flowing, want %q", got.Value, message.ConnectorStatusConnectedAndData)
			}
			connectedReports++
		case <-deadline:
			t.Fatalf("got %d ConnectedAndData reports, want the healthy state re-announced periodically", connectedReports)
		}
	}
}

// TestProcessSurvivesANonPositiveTimeout covers a config file that spells
// out `timeout: 0` - or anything negative. Both of process's timers are
// built from it and time.NewTicker panics on a non-positive interval, so
// this took the connector down at startup, which is the one outcome this
// whole status mechanism exists to prevent. It has to fall back to the
// default instead.
func TestProcessSurvivesANonPositiveTimeout(t *testing.T) {
	url := uniqueInprocURL(t)
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw]([]string{url}, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}

	recvCh := make(chan *message.Raw, 8)
	go sub.Receive(recvCh)
	warmUpPubSub(t, pub, recvCh)

	stream := make(chan []byte, 1)
	defer close(stream)
	go process(stream, "Ampero modules", "modbus", pub, 0)

	// config.DefaultConnectorTimeout is far longer than this test takes,
	// so the value below - not a timeout - is what produces both messages.
	stream <- []byte{0x01, 0x02, 0x03}

	first := receiveWithTimeout(t, recvCh)
	if first.Type != message.ConnectorStatusType || string(first.Value) != message.ConnectorStatusConnectedAndData {
		t.Fatalf("first message = %+v, want a ConnectedAndData status report", first)
	}
	second := receiveWithTimeout(t, recvCh)
	if second.Type != "modbus" || string(second.Value) != "\x01\x02\x03" {
		t.Fatalf("second message = %+v, want the real value", second)
	}
}
