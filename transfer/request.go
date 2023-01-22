package transfer

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mqtt"
	"go.uber.org/zap"
)

type TransferRequester struct {
	db                 *database.PostgresqlDatabase
	mqttConfig         *config.MQTTConfig
	mqttClient         *mqtt.Client
	origins            []config.OriginsConfig
	countRequestPeriod time.Duration
	dataRequestPeriod  time.Duration
	loadReduction      bool
}

func NewTransferRequester(c *config.TransferConfig) *TransferRequester {
	fmt.Println(c)
	return &TransferRequester{
		db:                 database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		mqttConfig:         &c.MQTTConfig,
		origins:            c.Origins,
		countRequestPeriod: c.CountRequestPeriod,
		dataRequestPeriod:  c.DataRequestPeriod,
		loadReduction:      c.LoadReduction,
	}
}

func (t *TransferRequester) Run() {
	rand.Seed(time.Now().UnixNano())

	t.mqttClient = mqtt.New(t.mqttConfig, t.messageReceived, fmt.Sprintf(respondTopic, "#"))
	defer t.mqttClient.Disconnect()

	// send count requests
	go func() {
		for {
			t.sendCountRequests(t.countRequestPeriod)
		}
	}()

	// send data requests
	go func() {
		for {
			t.sendDataRequests(t.dataRequestPeriod)
		}
	}()

	// never exit
	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait()
}

func (t *TransferRequester) sendCountRequests(interval time.Duration) {
	wg := new(sync.WaitGroup)
	wg.Add(len(t.origins) + 1)

	go func() {
		time.Sleep(interval)
		wg.Done()
	}()

	for _, origin := range t.origins {
		go func(origin string, epoch time.Time) {
			// wait random amount of time before processing to spread the workload
			if !t.loadReduction {
				time.Sleep(time.Duration(rand.Intn(int(interval))))
			}
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
			for period := epoch; period.Before(time.Now().Add(-2 * periodDuration)); period = period.Add(periodDuration) {
				if _, ok := completePeriods[period.UnixMicro()]; ok {
					continue // no need to send a count request because the period is already complete
				}
				if _, ok := incompletePeriods[period.UnixMicro()]; ok {
					continue // no need to send a count request because we already know the remote data points
				}
				t.sendMQTTCommand(origin, period, requestCountCmd, uuid.Nil)
			}
			wg.Done()
		}(origin.Origin, origin.Epoch)
	}
	wg.Wait()
}

func (t *TransferRequester) sendDataRequests(interval time.Duration) {
	wg := new(sync.WaitGroup)
	wg.Add(len(t.origins) + 1)
	go func() {
		time.Sleep(interval)
		wg.Done()
	}()

	for _, origin := range t.origins {
		go func(origin string) {
			// wait random amount of time before processing to spread the workload
			if !t.loadReduction {
				time.Sleep(time.Duration(rand.Intn(int(interval))))
			}

			incompletePeriods, err := t.db.SelectIncompletePeriods(origin)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not retrieve not completed periods from database",
					zap.String("Error", err.Error()),
				)
				return
			}
			for _, period := range incompletePeriods {
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
					uuid := uuid.New()
					timestamp := time.Now()
					t.db.LogTransferRequest(timestamp, uuid, origin, period, period.Add(periodDuration), localDataPoints, remoteDataPoints)
					t.db.UpdateStatistics(timestamp, origin, period, period.Add(periodDuration))
					t.sendMQTTCommand(origin, period, requestDataCmd, uuid)
				}
			}
			wg.Done()
		}(origin.Origin)
	}
	wg.Wait()
}

func (t *TransferRequester) responseReceived(origin string, response ResponseMessage) {
	t.db.CreatePeriod(origin, response.PeriodStart, response.PeriodStart.Add(periodDuration), response.DataPoints)
}

func (t *TransferRequester) sendMQTTCommand(origin string, start time.Time, command string, uuid uuid.UUID) {
	message := RequestMessage{
		Command:     command,
		PeriodStart: start,
		UUID:        uuid,
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
	t.mqttClient.Publish(topic, 0, true, bytes)
}

func (t *TransferRequester) messageReceived(c paho.Client, m paho.Message) {
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
