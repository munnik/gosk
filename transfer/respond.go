package transfer

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/uniqueue"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	noOfWorkers = 4
	queueSize   = 1_000_000 // should cover almost 10 years of periods
)

type TransferResponder struct {
	db                  *database.PostgresqlDatabase
	config              *config.TransferConfig
	mqttClient          *mqtt.Client
	publisher           mangos.Socket
	injectWorkerChannel *uniqueue.UQ[time.Time]

	uuidMap     map[int64]uuid.UUID
	uuidMapLock sync.RWMutex
}

func NewTransferResponder(c *config.TransferConfig) *TransferResponder {
	uuidMap := make(map[int64]uuid.UUID)
	return &TransferResponder{db: database.NewPostgresqlDatabase(&c.PostgresqlConfig), config: c, uuidMap: uuidMap, uuidMapLock: sync.RWMutex{}}
}

func (t *TransferResponder) Run(publisher mangos.Socket) {
	t.injectWorkerChannel = uniqueue.NewUQ[time.Time](queueSize)
	go t.startInjectDataWorkers()

	// listen for requests
	t.publisher = publisher
	t.mqttClient = mqtt.New(&t.config.MQTTConfig, t.messageReceived, fmt.Sprintf(requestTopic, t.config.Origin))
	defer t.mqttClient.Disconnect()

	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (t *TransferResponder) requestReceived(request RequestMessage) {
	switch request.Command {
	case requestCountCmd:
		t.respondWithCount(request.PeriodStart)

	case requestDataCmd:
		t.uuidMapLock.Lock()
		t.uuidMap[request.PeriodStart.Unix()] = request.UUID
		t.uuidMapLock.Unlock()
		t.injectWorkerChannel.Back() <- request.PeriodStart
	default:
		logger.GetLogger().Warn(
			"Unknown command in request",
			zap.String("Command", request.Command),
		)
	}
}

func (t *TransferResponder) respondWithCount(period time.Time) {
	count, err := t.db.ReadMappedCount(period, period.Add(periodDuration))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	response := ResponseMessage{
		DataPoints:  count,
		PeriodStart: period,
	}
	t.sendMQTTResponse(response)
}

func (t *TransferResponder) startInjectDataWorkers() {
	var wg sync.WaitGroup
	wg.Add(noOfWorkers)
	for i := 0; i < noOfWorkers; i++ {
		go func() {
			for period := range t.injectWorkerChannel.Front() {
				t.injectData(period)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func (t *TransferResponder) popUuid(period time.Time) uuid.UUID {
	t.uuidMapLock.RLock()
	defer t.uuidMapLock.RUnlock()

	uuid := t.uuidMap[period.Unix()]
	delete(t.uuidMap, period.Unix())

	return uuid
}

func (t *TransferResponder) injectData(period time.Time) {
	uuid := t.popUuid(period)
	deltas, err := t.db.ReadMapped(`WHERE "time" BETWEEN $1 AND $2`, period, period.Add(periodDuration))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	for j, delta := range deltas {
		if (j % t.config.SleepEveryN) == 0 {
			time.Sleep(t.config.SleepDuration)
		}
		for i := range delta.Updates {
			delta.Updates[i].Source.TransferUuid = uuid
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
	}

	// after sending the period it can be removed from the list of todo periods
	t.injectWorkerChannel.RemoveConstraint(period)
}

func (t *TransferResponder) sendMQTTResponse(message ResponseMessage) {
	bytes, err := json.Marshal(message)
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
	t.requestReceived(request)
}
