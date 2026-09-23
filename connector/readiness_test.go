package connector

import (
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

// deadAddress returns an address nothing is listening on.
func deadAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

// TestConnectorsPublishStatusWithoutTheirSensor guards the bug that made a
// missing sensor stall a deploy rather than merely report itself missing.
//
// Both of these connectors opened their connection in the constructor, which
// retries every 5 seconds until it succeeds - so with nothing on the other
// end the constructor never returned and Publish, and with it the process
// that publishes anything at all, was never reached. gosk signals readiness
// from its first published message, so the Type=notify unit stayed in
// "activating" forever, holding systemd's start job open and blocking
// switch-to-configuration. On node-rct-keizersgracht that held the system
// profile lock for 28 hours.
//
// What has to be true is simply that a connector whose sensor is absent still
// publishes its DisconnectedOrNoData status.
func TestConnectorsPublishStatusWithoutTheirSensor(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(*config.ConnectorConfig) (Connector[message.Raw], error)
	}{
		{"line", func(c *config.ConnectorConfig) (Connector[message.Raw], error) {
			return NewLineConnector(c)
		}},
		{"manner ethernet", func(c *config.ConnectorConfig) (Connector[message.Raw], error) {
			return NewMannerEthernetConnector(c)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			u, err := url.Parse("tcp://" + deadAddress(t))
			if err != nil {
				t.Fatalf("url.Parse: %v", err)
			}

			made := make(chan Connector[message.Raw], 1)
			go func() {
				connector, err := test.make(&config.ConnectorConfig{
					Name: "absent sensor", URL: u, Protocol: "nmea0183",
					DataBits: 8, StopBits: "1", Parity: "N",
					Timeout: 50 * time.Millisecond,
				})
				if err != nil {
					t.Errorf("constructor: %v", err)
					return
				}
				made <- connector
			}()

			var connector Connector[message.Raw]
			select {
			case connector = <-made:
			case <-time.After(5 * time.Second):
				t.Fatal("the constructor blocked waiting for a sensor that is not there")
			}

			url := uniqueInprocURL(t)
			pub := nanomsg.NewPublisher[message.Raw](url)
			sub, err := nanomsg.NewSubscriber[message.Raw]([]string{url}, []byte{})
			if err != nil {
				t.Fatalf("NewSubscriber: %v", err)
			}
			recvCh := make(chan *message.Raw, 8)
			go sub.Receive(recvCh)
			warmUpPubSub(t, pub, recvCh)

			go connector.Publish(pub)

			deadline := time.After(10 * time.Second)
			for {
				select {
				case got := <-recvCh:
					if got.Type != message.ConnectorStatusType {
						continue
					}
					if string(got.Value) != message.ConnectorStatusDisconnectedOrNoData {
						t.Fatalf("got status %q, want %q", got.Value, message.ConnectorStatusDisconnectedOrNoData)
					}
					return // published its status without ever reaching the sensor
				case <-deadline:
					t.Fatal("no status was published, so the unit would never become ready")
				}
			}
		})
	}
}
