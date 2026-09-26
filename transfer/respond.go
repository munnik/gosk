package transfer

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/jackc/pgtype"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/gosk/nanomsg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

const (
	bufferCapacity = 5000

	// countRequestQueueCapacity and dataRequestQueueCapacity bound how many
	// requests may wait for their worker. A full queue means the responder is
	// further behind than one request cycle, and the oldest entries would be
	// answered long after the requester gave up on them anyway, so requests
	// that do not fit are dropped instead of queued. The requester asks again
	// for every period that is still incomplete on its next cycle, so nothing
	// is lost permanently.
	//
	// The data queue holds one request cycle worth of periods per origin
	// (max_periods_to_request defaults to 500); counts are cheap to answer so
	// their queue is deeper.
	countRequestQueueCapacity = 2000
	dataRequestQueueCapacity  = 500
)

type TransferResponder struct {
	db                    *database.PostgresqlDatabase
	config                *config.TransferConfig
	mqttClient            *mqtt.Client
	sendBuffer            chan *message.Mapped
	countRequests         chan RequestMessage
	dataRequests          chan RequestMessage
	countRequestsReceived prometheus.Counter
	countRequestsHandled  prometheus.Counter
	countRequestsDropped  prometheus.Counter
	dataRequestsReceived  prometheus.Counter
	dataRequestsHandled   prometheus.Counter
	dataRequestsDropped   prometheus.Counter
	recordsTransmitted    prometheus.Counter
	uuidsTransmitted      prometheus.Counter
}

func NewTransferResponder(c *config.TransferConfig) *TransferResponder {
	result := &TransferResponder{
		db:                    database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:                c,
		countRequests:         make(chan RequestMessage, countRequestQueueCapacity),
		dataRequests:          make(chan RequestMessage, dataRequestQueueCapacity),
		countRequestsReceived: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_count_requests_received_total", Help: "total number of count requests received"}),
		countRequestsHandled:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_count_requests_handled_total", Help: "total number of count requests reponded to"}),
		countRequestsDropped:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_count_requests_dropped_total", Help: "total number of count requests dropped because the queue was full"}),
		dataRequestsReceived:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_data_requests_received_total", Help: "total number of data requests received"}),
		dataRequestsHandled:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_data_requests_handled_total", Help: "total number of data requests responded to"}),
		dataRequestsDropped:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_data_requests_dropped_total", Help: "total number of data requests dropped because the queue was full"}),
		recordsTransmitted:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_records_transmitted_total", Help: "total number of records sent again"}),
		uuidsTransmitted:      promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_uuids_transmitted_total", Help: "total number of uuids sent again"}),
	}

	promauto.NewGaugeFunc(
		prometheus.GaugeOpts{Name: "gosk_transfer_count_requests_queued", Help: "number of count requests waiting to be responded to"},
		func() float64 { return float64(len(result.countRequests)) },
	)
	promauto.NewGaugeFunc(
		prometheus.GaugeOpts{Name: "gosk_transfer_data_requests_queued", Help: "number of data requests waiting to be responded to"},
		func() float64 { return float64(len(result.dataRequests)) },
	)

	return result
}

func (t *TransferResponder) Run(publisher *nanomsg.Publisher[message.Mapped]) {
	// listen for requests
	t.sendBuffer = make(chan *message.Mapped, bufferCapacity)
	defer close(t.sendBuffer)
	go publisher.Send(t.sendBuffer)

	// Start the workers before subscribing, so a request that arrives on the
	// first connection is not dropped for want of a reader.
	go t.respondWithCountWorker()
	go t.respondWithDataWorker()

	t.mqttClient = mqtt.New(&t.config.MQTTConfig, t.messageReceived, fmt.Sprintf(requestTopic, t.config.Origin))
	defer t.mqttClient.Disconnect()

	// never exit
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}

// messageReceived only parses the request and hands it to a worker; it must not
// answer it. paho is configured with SetOrderMatters(false), so it dispatches
// every message in its own goroutine with no concurrency limit, and answering a
// data request inline meant one goroutine per request, each holding a full
// 5-minute window of mapped data resident while it slept
// sleep_between_respond_deltas between every delta. On origins with a large
// backfill backlog the requests arrived faster than the handlers retired: we
// measured 1982 of 2007 goroutines parked in that sleep at 8.5 GB of heap,
// which OOM-killed gosk 33 times in a week on a 16 GB machine and took the
// co-located postgres down with it.
//
// Draining each command from a single worker bounds the responder to one
// resident window, and makes sleep_between_respond_deltas an actual rate limit
// on the outgoing deltas - with unbounded concurrency it only decided how long
// each handler held its window, so raising it to protect a metered link grew
// memory without slowing the link down at all.
func (t *TransferResponder) messageReceived(c paho.Client, m paho.Message) {
	var request RequestMessage
	if err := json.Unmarshal(m.Payload(), &request); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}

	switch request.Command {
	case countCmd:
		t.countRequestsReceived.Inc()
		t.enqueueRequest(t.countRequests, request, t.countRequestsDropped)
	case dataCmd:
		t.dataRequestsReceived.Inc()
		t.enqueueRequest(t.dataRequests, request, t.dataRequestsDropped)
	default:
		logger.GetLogger().Warn(
			"Unknown command in request",
			zap.String("Command", request.Command),
		)
	}
}

// enqueueRequest queues a request for its worker, or drops it when the queue is
// full. It never blocks: a blocked callback is a paho goroutine we would keep
// for as long as the responder is behind, which is the pile-up this queue
// exists to prevent.
func (t *TransferResponder) enqueueRequest(queue chan<- RequestMessage, request RequestMessage, dropped prometheus.Counter) {
	select {
	case queue <- request:
	default:
		dropped.Inc()
		logger.GetLogger().Warn(
			"Request queue is full, dropping the request, the requester will ask again while the period is incomplete",
			zap.String("Command", request.Command),
			zap.String("UUID", request.UUID.String()),
			zap.Time("PeriodStart", request.PeriodStart),
		)
	}
}

func (t *TransferResponder) respondWithCountWorker() {
	for request := range t.countRequests {
		t.respondWithCount(request)
	}
}

func (t *TransferResponder) respondWithDataWorker() {
	for request := range t.dataRequests {
		t.respondWithData(request)
	}
}

func (t *TransferResponder) respondWithCount(request RequestMessage) {
	count, err := t.db.SelectCountMapped(t.config.Origin, request.PeriodStart)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	response := ResponseMessage{
		Command:     countCmd,
		DataPoints:  count,
		PeriodStart: request.PeriodStart,
		UUID:        request.UUID,
	}
	bytes, err := json.Marshal(response)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshall the response message",
			zap.String("Error", err.Error()),
		)
		return
	}
	topic := fmt.Sprintf(respondTopic, t.config.Origin)
	t.mqttClient.Publish(topic, 0, true, bytes)
	t.db.LogTransferRequest(t.config.Origin, response)
	t.countRequestsHandled.Inc()
}

func (t *TransferResponder) respondWithData(requestMessage RequestMessage) {
	localCountsPerUuid, err := t.db.SelectCountPerUuid(t.config.Origin, requestMessage.PeriodStart)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve counts per uuid from database",
			zap.String("Error", err.Error()),
			zap.String("Origin", t.config.Origin),
			zap.Time("Start", requestMessage.PeriodStart),
		)
		return
	}

	for uuid, count := range requestMessage.CountsPerUuid {
		if _, ok := localCountsPerUuid[uuid]; ok && (localCountsPerUuid[uuid] <= count) {
			// remove from list because remote already has complete set
			delete(localCountsPerUuid, uuid)
		}
	}

	requestMessage.CountsPerUuid = localCountsPerUuid
	t.injectData(requestMessage)
	t.db.LogTransferRequest(t.config.Origin, requestMessage)
	t.uuidsTransmitted.Add(float64(len(localCountsPerUuid)))
	t.dataRequestsHandled.Inc()
}

func (t *TransferResponder) injectData(requestMessage RequestMessage) {
	pgUuids := &pgtype.UUIDArray{}
	pgUuids.Set(uuidsOf(requestMessage.CountsPerUuid))

	deltas, err := t.db.ReadMapped(`WHERE "uuid" = ANY ($1) AND "time" BETWEEN $2 AND $2 + '5m'::interval`, pgUuids, requestMessage.PeriodStart)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	for _, delta := range deltas {
		for i := range delta.Updates {
			delta.Updates[i].Source.TransferUuid = requestMessage.UUID
		}
		t.sendBuffer <- delta
		t.recordsTransmitted.Inc()
		time.Sleep(t.config.SleepBetweenRespondDeltas)
	}
}

// uuidsOf returns the keys of countsPerUuid. The capacity, rather than the
// length, is what is preallocated: make([]uuid.UUID, len(...)) followed by
// append left the first half of the slice filled with the zero uuid, and every
// one of those was sent to postgres in the `"uuid" = ANY ($1)` array.
func uuidsOf(countsPerUuid map[uuid.UUID]int) []uuid.UUID {
	result := make([]uuid.UUID, 0, len(countsPerUuid))
	for uuid := range countsPerUuid {
		result = append(result, uuid)
	}
	return result
}
