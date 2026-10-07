package reader

import (
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
)

// newTestReader builds a reader without going through NewMqttReader, which
// registers prometheus collectors on the default registry and so can only be
// called once per test binary - readiness_test.go spends that one call.
// watchSource needs no more than these fields.
func newTestReader(timeout time.Duration) *MqttReader {
	return &MqttReader{
		mqttConfig:   &config.MQTTConfig{URLString: "tcp://broker.invalid:1883"},
		readerConfig: &config.ReaderConfig{Name: "fleet broker", Context: "servers.test", Timeout: timeout},
		sendBuffer:   make(chan *message.Mapped, 16),
		received:     make(chan struct{}, 1),
	}
}

// statusValue returns the single notification value the reader published,
// failing if what arrived is not shaped like one.
func statusValue(t *testing.T, r *MqttReader) message.SingleValueMapped {
	t.Helper()

	select {
	case got := <-r.sendBuffer:
		if got.Context != "servers.test" {
			t.Fatalf("got context %q, want the reader's configured context", got.Context)
		}
		values := got.ToSingleValueMapped()
		if len(values) != 1 {
			t.Fatalf("got %d values, want exactly one status value: %+v", len(values), got)
		}
		if values[0].Path != "notifications.readers.fleetbroker.connected" {
			t.Fatalf("got path %q, want notifications.readers.fleetbroker.connected", values[0].Path)
		}
		return values[0]
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the reader to publish a status")
		return message.SingleValueMapped{}
	}
}

// TestReaderNotifiesWhenItsSourceIsUnavailable is the reader's half of what
// connector/main.go's process does for a sensor: a source that is not
// producing data - an unreachable broker, or a fleet that has gone silent -
// has to become a SignalK notification. The reader published nothing at all
// in that situation before, so nothing downstream could tell a server that
// was receiving no data from one that was receiving none because it was
// broken.
//
// It must also keep repeating, for the same reason a connector's report does:
// nanomsg pub/sub has no replay, so a single alarm is missed entirely by
// anything that subscribed afterwards.
func TestReaderNotifiesWhenItsSourceIsUnavailable(t *testing.T) {
	r := newTestReader(50 * time.Millisecond)
	go r.watchSource()

	for i := 0; i < 3; i++ {
		value := statusValue(t, r)
		notification, ok := value.Value.(message.Notification)
		if !ok {
			t.Fatalf("report %d: got value %T (%v), want a message.Notification", i, value.Value, value.Value)
		}
		if notification.State == nil || *notification.State != "alarm" {
			t.Fatalf("report %d: got state %v, want an alarm", i, notification.State)
		}
		if notification.Message == nil || *notification.Message != "no data received from fleet broker" {
			t.Fatalf("report %d: got message %v, want it to name the reader", i, notification.Message)
		}
	}
}

// TestReaderClearsTheNotificationWhenDataReturns covers the other direction:
// a source that comes back has to clear the alarm it raised, or the server
// stays reported as broken for as long as it keeps working.
func TestReaderClearsTheNotificationWhenDataReturns(t *testing.T) {
	r := newTestReader(50 * time.Millisecond)
	go r.watchSource()

	// Wait out the first alarm, so the clearing report below is a real
	// transition rather than the initial state.
	if value := statusValue(t, r); value.Value == nil {
		t.Fatal("got a cleared notification before any data arrived, want the alarm first")
	}

	// What messageHandler signals for every message off the broker.
	r.received <- struct{}{}

	// More alarms may already be queued ahead of the signal, since the
	// source stayed quiet until this point.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("the alarm was never cleared after data returned")
		default:
		}
		if statusValue(t, r).Value == nil {
			return // cleared
		}
	}
}

// TestReaderSurvivesANonPositiveTimeout covers `timeout: 0` in a config
// file. watchSource builds a time.Ticker from it, which panics on a
// non-positive interval - taking the reader down over the very mechanism
// meant to keep it up.
func TestReaderSurvivesANonPositiveTimeout(t *testing.T) {
	r := newTestReader(0)
	go r.watchSource()

	// config.DefaultReaderTimeout is far longer than this test, so the
	// signal below - not a timeout - is what produces the only report.
	r.received <- struct{}{}

	if value := statusValue(t, r); value.Value != nil {
		t.Fatalf("got %+v, want the notification cleared once data arrived", value.Value)
	}
}

func TestBrokerNameNamesAnUnnamedReader(t *testing.T) {
	for _, test := range []struct{ url, want string }{
		// The dots and colons have to go: this name becomes one segment of a
		// SignalK path, where a dot is the segment separator, so leaving them
		// in would bury "connected" under a tree of hostname fragments
		// instead of naming a reader.
		{"mqtt://broker.mqtt.cool:1883", "broker-mqtt-cool-1883"},
		{"tcp://127.0.0.1:1883", "127-0-0-1-1883"},
		{"mqtt://hetzner-prod01.vpn.sustainablemotion.io:1883", "hetzner-prod01-vpn-sustainablemotion-io-1883"},
		// Not a url at all: naming the reader after the whole string beats
		// leaving it unnamed, which would publish under a path naming
		// nothing and collide with every other unnamed reader.
		{"not a url", "not a url"},
		{"", ""},
	} {
		if got := brokerName(test.url); got != test.want {
			t.Errorf("brokerName(%q) = %q, want %q", test.url, got, test.want)
		}
	}
}
