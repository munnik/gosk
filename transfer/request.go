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

type OriginPeriod struct {
	origin string
	period time.Time
}

type TransferRequester struct {
	db                        *database.PostgresqlDatabase
	mqttConfig                *config.MQTTConfig
	mqttClient                *mqtt.Client
	sleepBetweenCountRequests time.Duration
	sleepBetweenDataRequests  time.Duration
	numberOfRequestWorkers    int
	dataRequestChannel        chan OriginPeriod
}

func NewTransferRequester(c *config.TransferConfig) *TransferRequester {
	result := &TransferRequester{
		db:                        database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		mqttConfig:                &c.MQTTConfig,
		sleepBetweenCountRequests: c.SleepBetweenCountRequests,
		sleepBetweenDataRequests:  c.SleepBetweenDataRequests,
		numberOfRequestWorkers:    c.NumberOfRequestWorkers,
	}

	if result.numberOfRequestWorkers == 0 {
		result.numberOfRequestWorkers = 5
	}
	result.dataRequestChannel = make(chan OriginPeriod, result.numberOfRequestWorkers)

	return result
}

func (t *TransferRequester) Run() {
	rand.Seed(time.Now().UnixNano())

	t.mqttClient = mqtt.New(t.mqttConfig, t.messageReceived, fmt.Sprintf(respondTopic, "#"))
	defer t.mqttClient.Disconnect()

	// send count requests
	go func() {
		for {
			t.sendCountRequests()
		}
	}()

	// send data requests
	go func() {
		for i := 0; i < t.numberOfRequestWorkers; i++ {
			go t.sendDataRequestWorker(t.dataRequestChannel)
		}
		for {
			t.sendDataRequests()
		}
	}()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	wg.Wait() // never exit
}

func (t *TransferRequester) sendCountRequests() {
	origins, err := t.db.SelectFirstMappedDataPerOrigin()
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve first mapped data per origin, aborting count request",
			zap.String("Error", err.Error()),
			zap.Time("NextRequestAt", time.Now().Add(t.sleepBetweenCountRequests)),
		)

		time.Sleep(t.sleepBetweenCountRequests)
		return
	}

	minStart := time.Now()
	for _, start := range origins {
		if start.Before(minStart) {
			minStart = start
		}
	}

	existingRemoteCounts, err := t.db.SelectExistingRemoteCounts(minStart)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve existing remote counts, aborting count request",
			zap.String("Error", err.Error()),
			zap.Time("NextRequestAt", time.Now().Add(t.sleepBetweenCountRequests)),
		)

		time.Sleep(t.sleepBetweenCountRequests)
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(origins) + 1)

	go func() {
		time.Sleep(t.sleepBetweenCountRequests)
		wg.Done()
	}()

	for origin, start := range origins {
		go func(origin string, start time.Time) {
			// wait random amount of time before processing to spread the workload
			time.Sleep(time.Duration(rand.Intn(int(t.sleepBetweenCountRequests))))

			periods := make([]time.Time, 0)
			for p := start; p.Before(time.Now().Add(-countRequestCoolDown)); p = p.Add(periodDuration) {
				if _, ok := existingRemoteCounts[origin]; !ok {
					// no remote counts at all for origin so add this period
					periods = append(periods, p)
					continue
				}
				if _, ok := existingRemoteCounts[origin][p]; !ok {
					// for this period there is no remote count so add this period
					periods = append(periods, p)
					continue
				}
			}

			for _, period := range periods {
				requestMessage := RequestMessage{
					Command:     countCmd,
					UUID:        uuid.New(),
					PeriodStart: period,
				}
				t.sendMQTTCommand(origin, requestMessage)
				t.db.LogTransferRequest(origin, requestMessage)
			}
			wg.Done()
		}(origin, start)
	}

	wg.Wait()
}

func (t *TransferRequester) countResponseReceived(origin string, response ResponseMessage) {
	t.db.CreateRemoteCount(response.PeriodStart, origin, response.DataPoints)
	t.db.LogTransferRequest(origin, response)
}

func (t *TransferRequester) sendDataRequests() {
	origins, err := t.db.SelectIncompletePeriods()
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve incomplete periods per origin",
			zap.String("Error", err.Error()),
			zap.Time("NextRequestAt", time.Now().Add(t.sleepBetweenDataRequests)),
		)

		time.Sleep(t.sleepBetweenDataRequests)
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		time.Sleep(t.sleepBetweenDataRequests)
		wg.Done()
	}()

	for origin, periods := range origins {
		go func(origin string, periods []time.Time) {
			for _, period := range periods {
				t.dataRequestChannel <- OriginPeriod{origin: origin, period: period}
			}
		}(origin, periods)
	}
	wg.Wait()
}

func (t *TransferRequester) sendDataRequestWorker(dataRequests <-chan OriginPeriod) {
	for request := range dataRequests {
		countsPerUuid, err := t.db.SelectCountPerUuid(request.origin, request.period)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not retrieve counts per uuid from database",
				zap.String("Error", err.Error()),
				zap.String("Origin", request.origin),
				zap.Time("Start", request.period),
			)
			return
		}

		requestMessage := RequestMessage{
			Command:       dataCmd,
			UUID:          uuid.New(),
			PeriodStart:   request.period,
			CountsPerUuid: countsPerUuid,
		}
		t.sendMQTTCommand(request.origin, requestMessage)
		t.db.LogTransferRequest(request.origin, requestMessage)
	}
}

func (t *TransferRequester) sendMQTTCommand(origin string, message RequestMessage) {
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
	t.countResponseReceived(strings.TrimPrefix(m.Topic(), fmt.Sprintf(respondTopic, "")), response)
}
