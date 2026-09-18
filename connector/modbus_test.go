package connector

import (
	"net/url"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
)

// TestModbusConnectorReceiveBlocksWithNoRegisterGroups guards a real
// CPU-burning bug: with no register groups configured, receive had
// nothing for wg.Wait to wait on and returned immediately, so Publish's
// `for { m.receive(stream) }` spun as fast as the scheduler allowed -
// observed live burning more than a full CPU core per idle connector.
// See receive's doc comment for why it must block instead.
func TestModbusConnectorReceiveBlocksWithNoRegisterGroups(t *testing.T) {
	u, err := url.Parse("tcp://127.0.0.1:1")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	c := &config.ConnectorConfig{
		Name:     "test",
		URL:      u,
		StopBits: "1",
		Parity:   "N",
	}
	conn, err := NewModbusConnector(c, nil)
	if err != nil {
		t.Fatalf("NewModbusConnector: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_ = conn.receive(make(chan []byte, 1))
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("receive returned immediately instead of blocking with no register groups configured")
	case <-time.After(200 * time.Millisecond):
		// still blocked, as expected
	}
}
