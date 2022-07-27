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
	t.SendQueries()
}

func (t *TransferPublisher) messageReceived(c mqtt.Client, m mqtt.Message) {
	var command message.TransferMessage
	if err := json.Unmarshal(m.Payload(), &command); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	count, err := t.db.ReadMappedCount(appendToQuery, command.Origin, command.PeriodStart.Format(time.RFC3339), command.PeriodEnd.Format(time.RFC3339))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	if count != command.RemoteDataPoints {
		t.sendCommand(command.Origin, command.PeriodStart, command.PeriodEnd)
	}
	fmt.Printf("vessel: %d, server: %d\n", command.RemoteDataPoints, count)
	t.db.UpdateRemoteDataRemotePoints(command)

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
func (t *TransferPublisher) SendQueries() {
	origins, err := t.db.ReadRemoteOrigins()
	if err != nil {
		logger.GetLogger().Warn(err.Error())
	}

	for _, origin := range origins {
		appendToQuery := `WHERE "origin" = $1 ORDER BY "start"`
		periods, err := t.db.ReadRemoteData(appendToQuery, origin)
		if err != nil {
			logger.GetLogger().Warn(err.Error())
		}
		for _, period := range periods {
			if period.RemoteDataPoints > period.LocalDataPoints {
				appendToQuery := `WHERE "origin" = $1 AND "time" > $2 AND "time" < $3`
				local, err := t.db.ReadMappedCount(appendToQuery, origin, period.PeriodStart, period.PeriodEnd)
				if err != nil {
					logger.GetLogger().Warn(err.Error())
				}
				if local > period.LocalDataPoints {
					period.LocalDataPoints = local
					t.db.UpdateRemoteDataLocalPoints(period)
				} else {
					fmt.Println("reupdate")
					t.sendCommand(period.Origin, period.PeriodStart, period.PeriodEnd)
				}
				fmt.Println("incomplete")
				fmt.Println(period)
			}

		}
		threshold := time.Now().Add(-5 * time.Minute)
		last := periods[len(periods)-1].PeriodEnd
		fmt.Println(last)
		fmt.Println(len(periods))
		for last.Before(threshold) {
			last = last.Add(5 * time.Minute)
			t.sendQuery(origin, last, 5*time.Minute)
		}

	}
	// start, _ := time.Parse(time.RFC3339, "2022-01-01T00:00:00Z")
	// t.sendQuery("vessels.urn:mrn:imo:mmsi:244770688", start, 5*time.Minute)
}
func (t *TransferPublisher) sendQuery(origin string, start time.Time, length time.Duration) {
	message := CommandMessage{Command: QueryCmd}
	message.Request.Origin = origin
	message.Request.PeriodStart = start
	end := start.Add(length)
	message.Request.PeriodEnd = end
	bytes, err := json.Marshal(message)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshall the deltas",
			zap.String("Error", err.Error()),
		)
		return
	}
	topic := fmt.Sprintf(readTopic, origin)
	if token := t.mqttClient.Publish(topic, 1, false, bytes); token.Wait() && token.Error() != nil {
		logger.GetLogger().Warn(
			"Could not publish a message via MQTT",
			zap.String("Error", token.Error().Error()),
			zap.ByteString("Bytes", bytes),
		)
	}

	count, err := t.db.ReadMappedCount(appendToQuery, origin, start, end)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	message.Request.LocalDataPoints = count
	t.db.InsertRemoteData(message.Request)

}

func (t *TransferPublisher) sendCommand(origin string, start time.Time, end time.Time) {
	request := message.TransferMessage{Origin: origin,
		PeriodStart: start,
		PeriodEnd:   end,
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
	topic := fmt.Sprintf(readTopic, origin)
	if token := t.mqttClient.Publish(topic, 1, true, bytes); token.Wait() && token.Error() != nil {
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
