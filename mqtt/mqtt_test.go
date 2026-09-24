package mqtt

import (
	"net"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
)

// TestNewSurvivesABrokerThatIsNotThere guards the bug that made a restarting
// broker fail a deploy.
//
// New used to call logger.Fatal when the first connection failed, so anything
// that started while the broker was down exited 1. Deploying the mosquitto
// config to hetzner-prod01 restarts the broker, gosk-transferRequest2 started
// inside that window and died, and switch-to-configuration rolled the whole
// activation back - even though systemd restarted the unit successfully five
// seconds later.
func TestNewSurvivesABrokerThatIsNotThere(t *testing.T) {
	// an address nothing is listening on
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	address := listener.Addr().String()
	listener.Close()

	done := make(chan *Client, 1)
	go func() {
		done <- New(&config.MQTTConfig{
			URLString: "tcp://" + address,
			Username:  "test",
			Password:  "test",
		}, "test", nil, "")
	}()

	select {
	case client := <-done:
		if client == nil {
			t.Fatal("New returned nil for a broker that is merely absent")
		}
	case <-time.After(initialConnectWait + 20*time.Second):
		t.Fatal("New never returned for a broker that is not there")
	}
}
