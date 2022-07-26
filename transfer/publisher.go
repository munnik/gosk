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
	"go.uber.org/zap"
)

type TransferPublisher struct {
	db         *database.PostgresqlDatabase
	mqttConfig *config.MQTTConfig
	mqttClient mqtt.Client
}

func NewTransferPublisher(c *config.TransferConfig) *TransferPublisher {

	return &TransferPublisher{db: database.NewPostgresqlDatabase(&c.DBConfig), mqttConfig: &c.MQTTConfig}
}

func (t *TransferPublisher) createClientOptions() *mqtt.ClientOptions {
	o := mqtt.NewClientOptions()
	o.AddBroker(t.mqttConfig.URLString)
	o.SetCleanSession(true) // TODO: verify
	o.SetUsername(t.mqttConfig.Username)
	o.SetPassword(t.mqttConfig.Password)
	o.SetOrderMatters(false)
	o.SetKeepAlive(keepAlive)
	o.SetDefaultPublishHandler(t.messageReceived)
	o.SetConnectionLostHandler(t.disconnectHandler)
	o.SetOnConnectHandler(t.connectHandler)

	return o
}

func (t *TransferPublisher) connectHandler(c mqtt.Client) {
	logger.GetLogger().Info(
		"MQTT connection established",
	)

	topic := fmt.Sprintf(replyTopic, "#")
	if token := t.mqttClient.Subscribe(topic, 1, nil); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the MQTT topic",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", t.mqttConfig.URLString),
		)
		return
	}
}

func (t *TransferPublisher) messageReceived(c mqtt.Client, m mqtt.Message) {
	var command TransferMessage
	if err := json.Unmarshal(m.Payload(), &command); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	fmt.Println(m.Topic())
	count, err := t.db.ReadMappedCount(appendToQuery, command.Origin, command.PeriodStart.Format(time.RFC3339), command.PeriodEnd.Format(time.RFC3339))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	if count != command.LocalDataPoints {
		request := TransferMessage{Origin: command.Origin,
			PeriodStart: command.PeriodStart,
			PeriodEnd:   command.PeriodEnd,
		}
		reply := CommandMessage{Command: RequestCmd, Request: request}
		bytes, err := json.Marshal(reply)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not marshall the deltas",
				zap.String("Error", err.Error()),
			)
			return
		}
		topic := fmt.Sprintf(readTopic, command.Origin)
		if token := t.mqttClient.Publish(topic, 1, true, bytes); token.Wait() && token.Error() != nil {
			logger.GetLogger().Warn(
				"Could not publish a message via MQTT",
				zap.String("Error", token.Error().Error()),
				zap.ByteString("Bytes", bytes),
			)
		}
	}
	fmt.Printf("vessel: %d, server: %d\n", command.LocalDataPoints, count)

}
func (t *TransferPublisher) disconnectHandler(c mqtt.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}

}

func (t *TransferPublisher) ListenCountReply() {
	t.mqttClient = mqtt.NewClient(t.createClientOptions())
	if token := t.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the MQTT broker",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", t.mqttConfig.URLString),
		)
		return
	}
	defer t.mqttClient.Disconnect(disconnectWait)
	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (t *TransferPublisher) sendRequest(origin string, start time.Time) {
	message := CommandMessage{Command: QueryCmd}
	message.Request.Origin = origin
	message.Request.PeriodStart = start
	message.Request.PeriodEnd = start.Add(time.Minute * 5)
	bytes, err := json.Marshal(message)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshall the deltas",
			zap.String("Error", err.Error()),
		)
		return
	}
	topic := fmt.Sprintf(readTopic, origin)
	fmt.Println(topic)
	if token := t.mqttClient.Publish(topic, 1, false, bytes); token.Wait() && token.Error() != nil {
		logger.GetLogger().Warn(
			"Could not publish a message via MQTT",
			zap.String("Error", token.Error().Error()),
			zap.ByteString("Bytes", bytes),
		)
	}

}

// query database at interval
// ask clients via mqtt

// check for successful transfer after timeout
// mark completed in db
