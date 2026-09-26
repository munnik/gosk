package transfer

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestUuidsOfPreallocatesCapacityNotLength(t *testing.T) {
	countsPerUuid := map[uuid.UUID]int{
		uuid.New(): 1,
		uuid.New(): 2,
		uuid.New(): 3,
	}

	uuids := uuidsOf(countsPerUuid)

	if len(uuids) != len(countsPerUuid) {
		t.Fatalf("got %d uuids, want %d", len(uuids), len(countsPerUuid))
	}
	for _, u := range uuids {
		if u == uuid.Nil {
			// A zero uuid here means make() was given a length instead of a
			// capacity: every one of them ends up in the `"uuid" = ANY ($1)`
			// array sent to postgres.
			t.Errorf("uuids contains the zero uuid: %v", uuids)
		}
		if _, ok := countsPerUuid[u]; !ok {
			t.Errorf("uuid %v is not a key of countsPerUuid", u)
		}
	}
}

// TestEnqueueRequestDropsWhenFull is the memory bound from issue #18: paho
// dispatches every message in its own goroutine, so the callback may never
// block waiting for a worker, however far behind that worker is.
func TestEnqueueRequestDropsWhenFull(t *testing.T) {
	queue := make(chan RequestMessage, 2)
	dropped := prometheus.NewCounter(prometheus.CounterOpts{Name: "dropped"})
	responder := &TransferResponder{}

	for i := 0; i < 5; i++ {
		done := make(chan struct{})
		go func() {
			responder.enqueueRequest(queue, RequestMessage{Command: dataCmd, PeriodStart: time.Unix(int64(i), 0)}, dropped)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("enqueueRequest blocked on request %d", i)
		}
	}

	if len(queue) != 2 {
		t.Errorf("queue holds %d requests, want 2", len(queue))
	}
	if got := testutil.ToFloat64(dropped); got != 3 {
		t.Errorf("dropped %v requests, want 3", got)
	}
}

func TestMessageReceivedQueuesPerCommand(t *testing.T) {
	responder := &TransferResponder{
		countRequests:         make(chan RequestMessage, 1),
		dataRequests:          make(chan RequestMessage, 1),
		countRequestsReceived: prometheus.NewCounter(prometheus.CounterOpts{Name: "count_received"}),
		countRequestsDropped:  prometheus.NewCounter(prometheus.CounterOpts{Name: "count_dropped"}),
		dataRequestsReceived:  prometheus.NewCounter(prometheus.CounterOpts{Name: "data_received"}),
		dataRequestsDropped:   prometheus.NewCounter(prometheus.CounterOpts{Name: "data_dropped"}),
	}

	for _, command := range []string{countCmd, dataCmd, "nonsense"} {
		bytes, err := json.Marshal(RequestMessage{Command: command, UUID: uuid.New()})
		if err != nil {
			t.Fatalf("marshal %s request: %v", command, err)
		}
		responder.messageReceived(nil, testMessage(bytes))
	}

	if len(responder.countRequests) != 1 {
		t.Errorf("count queue holds %d requests, want 1", len(responder.countRequests))
	}
	if len(responder.dataRequests) != 1 {
		t.Errorf("data queue holds %d requests, want 1", len(responder.dataRequests))
	}
	if got := testutil.ToFloat64(responder.countRequestsDropped); got != 0 {
		t.Errorf("dropped %v count requests, want 0", got)
	}
	if got := testutil.ToFloat64(responder.dataRequestsDropped); got != 0 {
		t.Errorf("dropped %v data requests, want 0", got)
	}
}

// testMessage is the part of paho.Message messageReceived uses.
type testMessage []byte

func (m testMessage) Duplicate() bool   { return false }
func (m testMessage) Qos() byte         { return 0 }
func (m testMessage) Retained() bool    { return false }
func (m testMessage) Topic() string     { return fmt.Sprintf(requestTopic, "test") }
func (m testMessage) MessageID() uint16 { return 0 }
func (m testMessage) Payload() []byte   { return m }
func (m testMessage) Ack()              {}
