package connector

import (
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/protocol"
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

// absentSensorConfig is a ConnectorConfig pointing at urlString, which
// nothing answers on, with a timeout short enough to keep a test quick.
func absentSensorConfig(t *testing.T, urlString string) *config.ConnectorConfig {
	t.Helper()
	u, err := url.Parse(urlString)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", urlString, err)
	}
	return &config.ConnectorConfig{
		Name: "absent sensor", URL: u, URLString: urlString, Protocol: "nmea0183",
		BaudRate: 4800, DataBits: 8, StopBits: "1", Parity: "N",
		Timeout: 50 * time.Millisecond,
	}
}

// TestConnectorsPublishStatusWithoutTheirSensor guards the bug that made a
// missing sensor stall a deploy rather than merely report itself missing.
//
// Both the line and manner ethernet connectors opened their connection in
// the constructor, which retries every 5 seconds until it succeeds - so with
// nothing on the other end the constructor never returned, and Publish, and
// with it the process that publishes anything at all, was never reached.
// gosk's Type=notify units stayed in "activating" forever, holding systemd's
// start job open and blocking switch-to-configuration. On
// node-rct-keizersgracht that held the system profile lock for 28 hours.
//
// What has to be true, for every connector, is simply that one whose sensor
// is absent still publishes its DisconnectedOrNoData status - which is both
// what a mapper turns into an offline notification (see
// mapper.NewConnectorStatusUpdate) and proof the connector got as far as
// running process at all. Every connector is covered here rather than only
// the two that had the bug, because "the constructor quietly waits for the
// sensor" is not a mistake that stays fixed on its own.
func TestConnectorsPublishStatusWithoutTheirSensor(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(*testing.T) (Connector[message.Raw], error)
	}{
		{"line", func(t *testing.T) (Connector[message.Raw], error) {
			return NewLineConnector(absentSensorConfig(t, "tcp://"+deadAddress(t)))
		}},
		{"manner ethernet", func(t *testing.T) (Connector[message.Raw], error) {
			return NewMannerEthernetConnector(absentSensorConfig(t, "tcp://"+deadAddress(t)))
		}},
		{"modbus", func(t *testing.T) (Connector[message.Raw], error) {
			return NewModbusConnector(
				absentSensorConfig(t, "tcp://"+deadAddress(t)),
				[]config.RegisterGroupConfig{{
					ModbusHeader: protocol.ModbusHeader{
						Slave: 1, FunctionCode: protocol.ReadInputRegisters,
						Address: 1043, NumberOfCoilsOrRegisters: 1,
					},
					PollingInterval: 50 * time.Millisecond,
				}},
			)
		}},
		{"http", func(t *testing.T) (Connector[message.Raw], error) {
			address := deadAddress(t)
			return NewHttpConnector(
				absentSensorConfig(t, "tcp://"+address),
				[]config.UrlGroupConfig{{
					Url:             "http://" + address + "/status",
					PollingInterval: 50 * time.Millisecond,
				}},
			)
		}},
		{"mqtt", func(t *testing.T) (Connector[message.Raw], error) {
			address := deadAddress(t)
			c := absentSensorConfig(t, "tcp://"+address)
			return NewMQTTConnector(c, &config.MQTTConfig{
				URLString: "tcp://" + address,
				Topic:     "vessels/absent/#",
			})
		}},
		{"canbus", func(t *testing.T) (Connector[message.Raw], error) {
			// An interface name nothing on this machine can have: the
			// kernel caps them at 15 characters, so a longer one cannot
			// accidentally match a real CAN link on a developer's box.
			return NewCanBusConnector(absentSensorConfig(t, "can://gosk-no-such-can-interface"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			made := make(chan Connector[message.Raw], 1)
			go func() {
				connector, err := test.make(t)
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
