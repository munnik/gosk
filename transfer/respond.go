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

const bufferCapacity = 5000

type TransferResponder struct {
	db                    *database.PostgresqlDatabase
	config                *config.TransferConfig
	mqttClient            *mqtt.Client
	sendBuffer            chan *message.Mapped
	countRequestsReceived prometheus.Counter
	countRequestsHandled  prometheus.Counter
	dataRequestsReceived  prometheus.Counter
	dataRequestsHandled   prometheus.Counter
	recordsTransmitted    prometheus.Counter
	uuidsTransmitted      prometheus.Counter
}

func NewTransferResponder(c *config.TransferConfig) *TransferResponder {
	return &TransferResponder{
		db:                    database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:                c,
		countRequestsReceived: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_count_requests_received_total", Help: "total number of count requests received"}),
		countRequestsHandled:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_count_requests_handled_total", Help: "total number of count requests reponded to"}),
		dataRequestsReceived:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_data_requests_received_total", Help: "total number of data requests received"}),
		dataRequestsHandled:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_data_requests_handled_total", Help: "total number of data requests responded to"}),
		recordsTransmitted:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_records_transmitted_total", Help: "total number of records sent again"}),
		uuidsTransmitted:      promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_transfer_uuids_transmitted_total", Help: "total number of uuids sent again"}),
	}
}

func (t *TransferResponder) Run(publisher *nanomsg.Publisher[message.Mapped]) {
	// listen for requests
	t.sendBuffer = make(chan *message.Mapped, bufferCapacity)
	go publisher.Send(t.sendBuffer)
	t.mqttClient = mqtt.New(&t.config.MQTTConfig, t.messageReceived, fmt.Sprintf(requestTopic, t.config.Origin))
	defer t.mqttClient.Disconnect()

	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

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
		t.respondWithCount(request)
	case dataCmd:
		t.dataRequestsReceived.Inc()
		t.respondWithData(request)
	default:
		logger.GetLogger().Warn(
			"Unknown command in request",
			zap.String("Command", request.Command),
		)
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
	t.countRequestsHandled.Inc()
}

func (t *TransferResponder) respondWithData(request RequestMessage) {
	localCountsPerUuid, err := t.db.SelectCountPerUuid(t.config.Origin, request.PeriodStart)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve counts per uuid from database",
			zap.String("Error", err.Error()),
			zap.String("Origin", t.config.Origin),
			zap.Time("Start", request.PeriodStart),
		)
		return
	}

	for uuid, count := range request.CountsPerUuid {
		if _, ok := localCountsPerUuid[uuid]; ok && (localCountsPerUuid[uuid] <= count) {
			// remove from list because remote already has complete set
			delete(localCountsPerUuid, uuid)
		}
	}

	t.injectData(localCountsPerUuid, request.UUID, request.PeriodStart)
	t.uuidsTransmitted.Add(float64(len(localCountsPerUuid)))
	t.dataRequestsHandled.Inc()
}

func (t *TransferResponder) injectData(uuidsToTransmit map[uuid.UUID]int, transferUuid uuid.UUID, period time.Time) {
	uuids := make([]uuid.UUID, len(uuidsToTransmit))
	for uuid := range uuidsToTransmit {
		uuids = append(uuids, uuid)
	}
	pgUuids := &pgtype.UUIDArray{}
	pgUuids.Set(uuids)

	deltas, err := t.db.ReadMapped(`WHERE "uuid" = ANY ($1) AND "time" BETWEEN $2 AND $2 + '5m'::interval`, pgUuids, period)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	for _, delta := range deltas {
		for i := range delta.Updates {
			delta.Updates[i].Source.TransferUuid = transferUuid
		}
		t.sendBuffer <- delta
		t.recordsTransmitted.Inc()
		time.Sleep(t.config.SleepBetweenRespondDeltas)
	}
}
