package transfer

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
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

func (t *TransferRequester) Run() {
	// listen for responses
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

	// send count requests
	go func() {
		for {
			t.sendCountRequests()
		}
	}()

	// send data requests
	go func() {
		for {
			t.sendDataRequests()
		}
	}()
	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (t *TransferRequester) sendCountRequests() {
	rand.Seed(time.Now().UnixNano())

	wg := new(sync.WaitGroup)
	wg.Add(len(t.origins))

	threshold := time.Now().Add(-periodDuration)

	for _, origin := range t.origins {
		go func(origin string, epoch time.Time) {
			// wait random amount of time before processing to spread the workload
			time.Sleep(time.Duration(rand.Intn(int(30 * time.Minute))))

			completePeriods, err := t.db.SelectCompletePeriods(origin)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not retrieve completed periods from database",
					zap.String("Error", err.Error()),
				)
				return
			}
			incompletePeriods, err := t.db.SelectIncompletePeriods(origin)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not retrieve completed periods from database",
					zap.String("Error", err.Error()),
				)
				return
			}

			for period := epoch; period.Before(threshold); period = period.Add(periodDuration) {
				if _, ok := completePeriods[period]; ok {
					continue // no need to send a count request because the period is already complete
				}
				if _, ok := incompletePeriods[period]; ok {
					continue // no need to send a count request because we already now the remote data points
				}

				t.sendMQTTCommand(origin, period, requestCountCmd)
			}
			wg.Done()
		}(origin.Origin, origin.Epoch)
	}
	wg.Wait()
}

func (t *TransferRequester) sendDataRequests() {
	rand.Seed(time.Now().UnixNano())

	for _, origin := range t.origins {
		go func(origin string) {
			// wait random amount of time before processing to spread the workload
			time.Sleep(time.Duration(rand.Intn(int(2 * time.Hour))))

			incompletePeriods, err := t.db.SelectIncompletePeriods(origin)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not retrieve not completed periods from database",
					zap.String("Error", err.Error()),
				)
				return
			}
			for period := range incompletePeriods {
				localDataPoints, remoteDataPoints, err := t.db.SelectLocalAndRemoteDataPoints(origin, period, period.Add(periodDuration))
				if err != nil {
					logger.GetLogger().Warn(
						"Could not retrieve local and remote data points from database",
						zap.String("Origin", origin),
						zap.Time("PeriodStart", period),
						zap.Time("PeriodEnd", period.Add(periodDuration)),
						zap.String("Error", err.Error()),
					)
					continue
				}
				t.db.UpdateLocalDataPoints(origin, period, period.Add(periodDuration), localDataPoints)
				// check if local data points are still less after update
				if localDataPoints < remoteDataPoints {
					t.sendMQTTCommand(origin, period, requestDataCmd)
				}
			}
		}(origin.Origin)
	}
}

func (t *TransferRequester) responseReceived(origin string, response ResponseMessage) {
	t.db.CreatePeriod(origin, response.PeriodStart, response.PeriodStart.Add(periodDuration), response.DataPoints)
}

func (t *TransferRequester) sendMQTTCommand(origin string, start time.Time, command string) {
	message := RequestMessage{
		Command:     command,
		PeriodStart: start,
	}
	bytes, err := json.Marshal(message)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshall the request message",
			zap.String("Error", err.Error()),
		)
		return
	}
	topic := fmt.Sprintf(requestTopic, origin)
	if token := t.mqttClient.Publish(topic, 0, true, bytes); token.Wait() && token.Error() != nil {
		logger.GetLogger().Warn(
			"Could not publish a message via MQTT",
			zap.String("Error", token.Error().Error()),
			zap.ByteString("Bytes", bytes),
		)
	}
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
	topic := fmt.Sprintf(respondTopic, "#")
	if token := t.mqttClient.Subscribe(topic, 1, nil); token.Wait() && token.Error() != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe to the MQTT topic",
			zap.String("Error", token.Error().Error()),
			zap.String("URL", t.mqttConfig.URLString),
		)
		return
	}
}

func (t *TransferRequester) disconnectHandler(c mqtt.Client, e error) {
	if e != nil {
		logger.GetLogger().Warn(
			"MQTT connection lost",
			zap.String("Error", e.Error()),
		)
	}
}

func (t *TransferRequester) messageReceived(c mqtt.Client, m mqtt.Message) {
	var response ResponseMessage
	if err := json.Unmarshal(m.Payload(), &response); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal buffer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	t.responseReceived(strings.TrimPrefix(m.Topic(), fmt.Sprintf(respondTopic, "")), response)
}
