package transfer

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	disconnectWait = 5 * time.Second
	keepAlive      = 30 * time.Second
)

type TransferResponder struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient mqtt.Client
	publisher  mangos.Socket
}

func NewTransferResponder(c *config.TransferConfig) *TransferResponder {
	return &TransferResponder{db: database.NewPostgresqlDatabase(&c.PostgresqlConfig), config: c}
}

func (t *TransferResponder) ReadCommands(publisher mangos.Socket) {
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
	topic := fmt.Sprintf(countData, t.config.Origin)
	if token := t.mqttClient.Subscribe(topic, 1, nil); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the MQTT topic",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", t.config.MQTTConfig.URLString),
		)
		return
	}
}

func (t *TransferResponder) messageReceived(c mqtt.Client, m mqtt.Message) {
	var command CommandMessage
	if err := json.Unmarshal(m.Payload(), &command); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}

	switch command.Command {
	case requestCountCmd:
		t.respondWithCount(command.Request)

	case requestDataCmd:
		t.injectData(command.Request)
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

func (t *TransferResponder) respondWithCount(request message.TransferRequest) {
	count, err := t.db.ReadMappedCount(betweenIntervalWhereClause, request.Origin, request.PeriodStart, request.PeriodEnd)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	reply := message.TransferRequest{
		Origin:           request.Origin,
		PeriodStart:      request.PeriodStart,
		PeriodEnd:        request.PeriodEnd,
		RemoteDataPoints: count,
	}
	bytes, err := json.Marshal(reply)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshall the deltas",
			zap.String("Error", err.Error()),
		)
		return
	}
	topic := fmt.Sprintf(replyTopic, t.config.Origin)
	if token := t.mqttClient.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		logger.GetLogger().Warn(
			"Could not publish a message via MQTT",
			zap.String("Error", token.Error().Error()),
			zap.ByteString("Bytes", bytes),
		)
	}
}

func (t *TransferResponder) injectData(request message.TransferRequest) {
	deltas, err := t.db.ReadMapped(betweenIntervalWhereClause, request.Origin, request.PeriodStart.Format(time.RFC3339), request.PeriodEnd.Format(time.RFC3339))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}

	for _, delta := range deltas {
		bytes, err := json.Marshal(delta)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not marshal delta",
				zap.String("Error", err.Error()),
			)
			continue
		}
		time.Sleep(100 * time.Millisecond) // slow down to stop db from crashing
		if err := t.publisher.Send(bytes); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
	}
}
