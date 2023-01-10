package transfer

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/uniqueue"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	disconnectWait = 5 * time.Second
	keepAlive      = 30 * time.Second
	noOfWorkers    = 4
	queueSize      = 1_000_000 // should cover almost 10 years of periods
)

type TransferResponder struct {
	db                  *database.PostgresqlDatabase
	config              *config.TransferConfig
	mqttClient          mqtt.Client
	publisher           mangos.Socket
	injectWorkerChannel *uniqueue.UQ[time.Time]
	uuidMap             map[int64]uuid.UUID
	uuidMapLock         sync.RWMutex
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
	t.mqttClient = mqtt.NewClient(t.createClientOptions())
	if token := t.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the MQTT broker",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", t.config.MQTTConfig.URLString),
		)
		return
	}
	defer t.mqttClient.Disconnect(uint(disconnectWait.Milliseconds()))

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
				t.uuidMapLock.RLock()
				uuid := t.uuidMap[period.Unix()]
				t.uuidMapLock.RUnlock()
				t.injectData(period, uuid)
				t.uuidMapLock.Lock()
				delete(t.uuidMap, period.Unix())
				t.uuidMapLock.Unlock()
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func (t *TransferResponder) injectData(period time.Time, uuid uuid.UUID) {
	deltas, err := t.db.ReadMapped(`WHERE "time" BETWEEN $1 AND $2`, period, period.Add(periodDuration))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}

	for _, delta := range deltas {
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
	if token := t.mqttClient.Publish(topic, 0, true, bytes); token.Wait() && token.Error() != nil {
		logger.GetLogger().Warn(
			"Could not publish a message via MQTT",
			zap.String("Error", token.Error().Error()),
			zap.ByteString("Bytes", bytes),
		)
	}
}

func (t *TransferResponder) createClientOptions() *mqtt.ClientOptions {
	o := mqtt.NewClientOptions()
	o.AddBroker(t.config.MQTTConfig.URLString)
	o.SetCleanSession(true) // TODO: verify
	o.SetUsername(t.config.MQTTConfig.Username)
	o.SetPassword(t.config.MQTTConfig.Password)
	o.SetOrderMatters(false)
	o.SetKeepAlive(keepAlive)
	o.SetDefaultPublishHandler(t.messageReceived)
	o.SetConnectionLostHandler(t.disconnectHandler)
	o.SetOnConnectHandler(t.connectHandler)

	return o
}

func (t *TransferResponder) connectHandler(c mqtt.Client) {
	logger.GetLogger().Info(
		"MQTT connection established",
	)
	topic := fmt.Sprintf(requestTopic, t.config.Origin)
	if token := t.mqttClient.Subscribe(topic, 1, nil); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the MQTT topic",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", t.config.MQTTConfig.URLString),
		)
		return
	}

}
func (t *TransferResponder) disconnectHandler(c mqtt.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}
}

func (t *TransferResponder) messageReceived(c mqtt.Client, m mqtt.Message) {
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
