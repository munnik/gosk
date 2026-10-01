package mapper

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
// listened on yet. A url spelled out as a constant only works once per test
// binary; see nanomsg's own uniqueInprocURL for why.
func uniqueInprocURL(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("inproc://%s-%d", t.Name(), inprocSeq.Add(1))
}

// silentMapper maps nothing, because nothing is ever sent to it.
type silentMapper struct{}

func (silentMapper) DoMap(*message.Mapped) (*message.Mapped, error) {
	return message.NewMapped(), nil
}

func (silentMapper) GetTickerInterval() time.Duration { return 0 }

// silentRawMapper is silentMapper for the Mapped->Raw direction.
type silentRawMapper struct{}

func (silentRawMapper) DoMap(*message.Mapped) (*message.Raw, error) {
	return message.NewRaw(), nil
}

// silentShard is silentMapper as a shard, for processSharded.
type silentShard struct{ silentMapper }

// TestMappersReportReadyWithoutAnyInput covers every way a mapper is run:
// each has to tell systemd the unit has started as soon as it is subscribed
// and publishing, without waiting for anything upstream to send it
// something.
//
// Readiness used to wait for the first message consumed, which looks
// harmless until the sensor at the head of the pipeline is absent. The only
// thing a connector publishes then is its DisconnectedOrNoData report, one
// config.ConnectorConfig.Timeout after it started - 30s by default, against
// a 40s TimeoutStartSec - so a connector timeout raised past
// TimeoutStartSec started the connect unit fine (connector/main.go's
// process reports ready immediately) but failed every map and notify unit
// behind it. Worse down a chain: nanomsg pub/sub has no replay, so a stage
// that came up just after a report was published waited out the whole
// interval again for the next one.
//
// processSharded reported no readiness of its own at all, leaving a sharded
// mapper on the publisher's first successful send - so one whose shards had
// nothing to say never became ready however correctly it ran.
func TestMappersReportReadyWithoutAnyInput(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(string, string)
	}{
		{"process", func(subscribeURL, publishURL string) {
			sub, err := nanomsg.NewSubscriber[message.Mapped]([]string{subscribeURL}, []byte{})
			if err != nil {
				t.Errorf("NewSubscriber: %v", err)
				return
			}
			process(sub, nanomsg.NewPublisher[message.Mapped](publishURL), silentMapper{}, true)
		}},
		{"processRaw", func(subscribeURL, publishURL string) {
			sub, err := nanomsg.NewSubscriber[message.Mapped]([]string{subscribeURL}, []byte{})
			if err != nil {
				t.Errorf("NewSubscriber: %v", err)
				return
			}
			processRaw(sub, nanomsg.NewPublisher[message.Raw](publishURL), silentRawMapper{})
		}},
		{"processSharded", func(subscribeURL, publishURL string) {
			sub, err := nanomsg.NewSubscriber[message.Mapped]([]string{subscribeURL}, []byte{})
			if err != nil {
				t.Errorf("NewSubscriber: %v", err)
				return
			}
			processSharded(
				sub,
				nanomsg.NewPublisher[message.Mapped](publishURL),
				[]shardedMapper{silentShard{}, silentShard{}},
				map[string]int{"environment.outside.pressure": 0, "propulsion.main.rpm": 1},
				true,
			)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			socketPath := filepath.Join(t.TempDir(), "notify.sock")
			listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
			if err != nil {
				t.Fatalf("could not listen on a test unixgram socket: %v", err)
			}
			defer listener.Close()
			t.Setenv("NOTIFY_SOCKET", socketPath)

			// Nothing ever publishes on the url this subscribes to, so a
			// READY datagram can only have come from the mapper starting.
			go test.run(uniqueInprocURL(t), uniqueInprocURL(t))

			listener.SetReadDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, 64)
			n, err := listener.Read(buf)
			if err != nil {
				t.Fatalf("expected a READY datagram from a mapper with no input, got error: %v", err)
			}
			if got := string(buf[:n]); got != "READY=1" {
				t.Fatalf("got %q, want %q", got, "READY=1")
			}
		})
	}
}
