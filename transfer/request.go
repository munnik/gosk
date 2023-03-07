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
	db                        *database.PostgresqlDatabase
	mqttConfig                *config.MQTTConfig
	mqttClient                *mqtt.Client
	countRequestSleepInterval time.Duration
	dataRequestSleepInterval  time.Duration
}

func NewTransferRequester(c *config.TransferConfig) *TransferRequester {
	fmt.Println(c)
	return &TransferRequester{
		db:                        database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		mqttConfig:                &c.MQTTConfig,
		countRequestSleepInterval: c.CountRequestSleepInterval,
		dataRequestSleepInterval:  c.DataRequestSleepInterval,
	}
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
			"Could not retrieve first mapped data per origin",
			zap.String("Error", err.Error()),
		)

		time.Sleep(t.countRequestSleepInterval)
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
			"Could not retrieve existing remote counts",
			zap.String("Error", err.Error()),
		)

		time.Sleep(t.countRequestSleepInterval)
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(origins) + 1)

	go func() {
		time.Sleep(t.countRequestSleepInterval)
		wg.Done()
	}()

	for origin, start := range origins {
		go func(origin string, start time.Time) {
			// wait random amount of time before processing to spread the workload
			time.Sleep(time.Duration(rand.Intn(int(t.countRequestSleepInterval))))

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
		)

		time.Sleep(t.dataRequestSleepInterval)
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(origins) + 1)

	go func() {
		time.Sleep(t.dataRequestSleepInterval)
		wg.Done()
	}()

	for origin, periods := range origins {
		go func(origin string, periods []time.Time) {
			for _, period := range periods {
				// wait random amount of time before processing to spread the workload
				time.Sleep(time.Duration(rand.Intn(int(t.dataRequestSleepInterval))))

				countsPerUuid, err := t.db.SelectCountPerUuid(origin, period)
				if err != nil {
					logger.GetLogger().Warn(
						"Could not retrieve counts per uuid from database",
						zap.String("Error", err.Error()),
						zap.String("Origin", origin),
						zap.Time("Start", period),
					)
					return
				}

				requestMessage := RequestMessage{
					Command:       dataCmd,
					UUID:          uuid.New(),
					PeriodStart:   period,
					CountsPerUuid: countsPerUuid,
				}
				t.sendMQTTCommand(origin, requestMessage)
				t.db.LogTransferRequest(origin, requestMessage)
			}
			wg.Done()
		}(origin, periods)
	}
	wg.Wait()
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
