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

type TransferRequester struct {
	db         *database.PostgresqlDatabase
	mqttConfig *config.MQTTConfig
	mqttClient mqtt.Client
	origins    []config.OriginsConfig
}

func NewTransferRequester(c *config.TransferConfig) *TransferRequester {
	return &TransferRequester{db: database.NewPostgresqlDatabase(&c.PostgresqlConfig), mqttConfig: &c.MQTTConfig, origins: c.Origins}
}

func (t *TransferRequester) createClientOptions() *mqtt.ClientOptions {
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

func (t *TransferRequester) connectHandler(c mqtt.Client) {
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

func (t *TransferRequester) messageReceived(c mqtt.Client, m mqtt.Message) {
	var request message.TransferRequest
	if err := json.Unmarshal(m.Payload(), &request); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	count, err := t.db.ReadMappedCount(betweenIntervalWhereClause, request.Origin, request.PeriodStart.Format(time.RFC3339), request.PeriodEnd.Format(time.RFC3339))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve count of mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	if count != request.RemoteDataPoints {
		t.requestData(request.Origin, request.PeriodStart, request.PeriodEnd)
	}
	// fmt.Printf("vessel: %d, server: %d\n", command.RemoteDataPoints, count)
	t.db.UpdateRemoteDataRemotePoints(request)
}

func (t *TransferRequester) disconnectHandler(c mqtt.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}
}

func (t *TransferRequester) ListenCountResponse() {
	t.mqttClient = mqtt.NewClient(t.createClientOptions())
	if token := t.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the MQTT broker",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", t.mqttConfig.URLString),
		)
		return
	}
	defer t.mqttClient.Disconnect(uint(disconnectWait.Milliseconds()))
	go func() {
		for {
			t.SendCountRequests()
			time.Sleep(1 * time.Hour)
		}
	}()
	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (t *TransferRequester) SendCountRequests() {
	wg := new(sync.WaitGroup)
	wg.Add(len(t.origins))
	for _, origin := range t.origins {
		go func(origin string, epoch time.Time) {
			periods, err := t.db.CreateTransferRequests(forOriginWhereClause, origin)
			if err != nil {
				logger.GetLogger().Warn(err.Error())
			}

			for _, period := range periods {
				if period.RemoteDataPoints > period.LocalDataPoints {
					local, err := t.db.ReadMappedCount(betweenIntervalWhereClause, origin, period.PeriodStart, period.PeriodEnd)
					if err != nil {
						logger.GetLogger().Warn(err.Error())
					}
					if local > period.LocalDataPoints {
						// extra values have come in since last check
						period.LocalDataPoints = local
						t.db.UpdateRemoteDataLocalPoints(period)
					} else {
						// ask for period to be resent
						t.requestData(period.Origin, period.PeriodStart, period.PeriodEnd)
						time.Sleep(30 * time.Second)
					}
				}
			}

			if len(periods) > 0 {
				// periods already exist
				threshold := time.Now().Add(-5 * time.Minute)
				last := periods[len(periods)-1].PeriodEnd
				for last.Before(threshold) {
					last = last.Add(5 * time.Minute)
					t.sendCountRequest(origin, last, 5*time.Minute)
					time.Sleep(1 * time.Second)
				}
			} else {
				t.sendCountRequest(origin, epoch, 5*time.Minute)
				time.Sleep(1 * time.Second)
			}
			wg.Done()
		}(origin.Origin, origin.Epoch)
	}
	wg.Wait()
}

func (t *TransferRequester) sendCountRequest(origin string, start time.Time, length time.Duration) {
	message := CommandMessage{Command: requestCountCmd}
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
	topic := fmt.Sprintf(countData, origin)
	if token := t.mqttClient.Publish(topic, 1, false, bytes); token.Wait() && token.Error() != nil {
		logger.GetLogger().Warn(
			"Could not publish a message via MQTT",
			zap.String("Error", token.Error().Error()),
			zap.ByteString("Bytes", bytes),
		)
	}

	count, err := t.db.ReadMappedCount(betweenIntervalWhereClause, origin, start, end)
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

func (t *TransferRequester) requestData(origin string, start time.Time, end time.Time) {
	request := message.TransferRequest{Origin: origin,
		PeriodStart: start,
		PeriodEnd:   end,
	}
	reply := CommandMessage{Command: requestDataCmd, Request: request}
	bytes, err := json.Marshal(reply)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshall the deltas",
			zap.String("Error", err.Error()),
		)
		return
	}
	topic := fmt.Sprintf(countData, origin)
	if token := t.mqttClient.Publish(topic, 1, true, bytes); token.Wait() && token.Error() != nil {
		logger.GetLogger().Warn(
			"Could not publish a message via MQTT",
			zap.String("Error", token.Error().Error()),
			zap.ByteString("Bytes", bytes),
		)
	}
}
