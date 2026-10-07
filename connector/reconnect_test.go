package connector

import (
	"net"
	"testing"
	"time"

	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

// TestListeningConnectorAcceptsASecondConnection covers a sensor that
// disconnects and comes back, on a connector configured to listen rather
// than dial.
//
// createNetworkConnection used to call net.Listen for every connection and
// drop the listener without closing it, so the second connection's
// net.Listen hit the port its own predecessor was still holding, failed with
// "address already in use", and went on failing every five seconds forever.
// The connector then reported DisconnectedOrNoData for the rest of the
// process's life - raising an offline notification for a sensor that was
// sitting right there, reconnecting, and could only be recovered by
// restarting the unit. See listener.
func TestListeningConnectorAcceptsASecondConnection(t *testing.T) {
	// deadAddress leaves this port free for the connector to bind.
	address := deadAddress(t)
	c := absentSensorConfig(t, "tcp://"+address)
	c.Listen = true
	// Long enough that no timeout fires during the test, so every status
	// report below would have to be a real one.
	c.Timeout = time.Minute

	connector, err := NewLineConnector(c)
	if err != nil {
		t.Fatalf("NewLineConnector: %v", err)
	}

	url := uniqueInprocURL(t)
	pub := nanomsg.NewPublisher[message.Raw](url)
	sub, err := nanomsg.NewSubscriber[message.Raw]([]string{url}, []byte{})
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	recvCh := make(chan *message.Raw, 32)
	go sub.Receive(recvCh)
	warmUpPubSub(t, pub, recvCh)

	go connector.Publish(pub)

	for _, sentence := range []string{"$GPGLL,first", "$GPGLL,second"} {
		conn := dialUntilAccepted(t, address)
		if _, err := conn.Write([]byte(sentence + "\r\n")); err != nil {
			t.Fatalf("writing %q: %v", sentence, err)
		}

		// Status reports are not what this test is about, so skip past them
		// to the sentence itself.
		deadline := time.After(10 * time.Second)
		for {
			select {
			case got := <-recvCh:
				if got.Type == message.ConnectorStatusType {
					continue
				}
				if string(got.Value) != sentence {
					t.Fatalf("got %q, want %q", got.Value, sentence)
				}
			case <-deadline:
				t.Fatalf("%q never arrived, so the connector never accepted this connection", sentence)
			}
			break
		}

		// Drop the connection the way a sensor losing power does, leaving
		// the connector to accept the next one.
		conn.Close()
	}
}

// dialUntilAccepted connects to address, retrying while nothing is
// listening yet - the connector binds its port from the goroutine Publish
// starts, so there is no moment the caller can synchronise on.
func dialUntilAccepted(t *testing.T, address string) net.Conn {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.Dial("tcp", address)
		if err == nil {
			t.Cleanup(func() { conn.Close() })
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatalf("the connector never listened on %v: %v", address, err)
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
}
