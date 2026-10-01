package reader

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

// TestMqttReaderReportsReadyWithoutAnyMessage covers the readiness this
// reader had none of its own.
//
// It used to rely entirely on nanomsg.Publisher's first successful send
// (see pub.go's send), which here needs some vessel to have published on
// mqttTopic and that payload to decompress, unmarshal and publish - none
// of which this process controls - with mqtt.New blocking for up to
// mqtt.initialConnectWait before any of it could even begin. On a quiet
// fleet, or against a broker that had just been restarted, the unit could
// stay below gosk.nix's TimeoutStartSec and be killed while running
// perfectly well. Readiness has to mean "this reader is up and
// subscribed", not "the fleet happened to say something in the first 40
// seconds".
func TestMqttReaderReportsReadyWithoutAnyMessage(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "notify.sock")
	notify, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		t.Fatalf("could not listen on a test unixgram socket: %v", err)
	}
	defer notify.Close()
	t.Setenv("NOTIFY_SOCKET", socketPath)

	// A broker address nothing answers on, so no message can ever arrive
	// and a READY datagram can only have come from the reader starting.
	// paho retries this in the background for the rest of the test.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	broker := probe.Addr().String()
	probe.Close()

	reader := NewMqttReader(&config.MQTTConfig{URLString: "tcp://" + broker, Compress: true})
	go reader.ReadMapped(nanomsg.NewPublisher[message.Mapped]("inproc://" + t.Name()))

	notify.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := notify.Read(buf)
	if err != nil {
		t.Fatalf("expected a READY datagram from a reader with no traffic, got error: %v", err)
	}
	if got := string(buf[:n]); got != "READY=1" {
		t.Fatalf("got %q, want %q", got, "READY=1")
	}
}
