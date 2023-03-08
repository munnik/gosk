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
	"github.com/munnik/gosk/mqtt"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type TransferResponder struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	publisher  mangos.Socket
}

func NewTransferResponder(c *config.TransferConfig) *TransferResponder {
	return &TransferResponder{
		db:     database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config: c,
	}
}

func (t *TransferResponder) Run(publisher mangos.Socket) {
	// listen for requests
	t.publisher = publisher
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
		t.respondWithCount(request)
	case dataCmd:
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
		bytes, err := json.Marshal(delta)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not marshal delta",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := t.publisher.Send(bytes); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
		time.Sleep(t.config.SleepBetweenRespondDeltas)
	}
}
