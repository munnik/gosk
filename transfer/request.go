package transfer

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mqtt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"golang.org/x/exp/rand"
)

type TransferRequester struct {
	db                        *database.PostgresqlDatabase
	mqttConfig                *config.MQTTConfig
	mqttClient                *mqtt.Client
	sleepBetweenCountRequests time.Duration
	sleepBetweenDataRequests  time.Duration
	numberOfRequestWorkers    int
	maxPeriodsToRequest       int
	completenessFactor        float64
	dataRequestChannel        chan database.IncompletePeriod
	countRequestsSent         prometheus.CounterVec
	countResponsesReceived    prometheus.CounterVec
	dataRequestsSent          prometheus.CounterVec
	dataMissingPeriods        prometheus.GaugeVec
	firstPeriodRequested      prometheus.GaugeVec
	lastPeriodRequested       prometheus.GaugeVec
}

func NewTransferRequester(c *config.TransferConfig) *TransferRequester {
	result := &TransferRequester{
		db:                        database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		mqttConfig:                &c.MQTTConfig,
		sleepBetweenCountRequests: c.SleepBetweenCountRequests,
		sleepBetweenDataRequests:  c.SleepBetweenDataRequests,
		numberOfRequestWorkers:    c.NumberOfRequestWorkers,
		maxPeriodsToRequest:       c.MaxPeriodsToRequest,
		completenessFactor:        c.CompletenessFactor,
		countRequestsSent:         *promauto.NewCounterVec(prometheus.CounterOpts{Name: "gosk_transfer_count_requests_total", Help: "total number of count requests sent, partitioned by origin"}, []string{"origin"}),
		countResponsesReceived:    *promauto.NewCounterVec(prometheus.CounterOpts{Name: "gosk_transfer_count_responses_total", Help: "total number of count responses received, partitioned by origin"}, []string{"origin"}),
		dataRequestsSent:          *promauto.NewCounterVec(prometheus.CounterOpts{Name: "gosk_transfer_data_requests_total", Help: "total number of data requests sent, partitioned by origin"}, []string{"origin"}),
		dataMissingPeriods:        *promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "gosk_transfer_missing_periods_total", Help: "total number of periods with missing data, partitioned by origin"}, []string{"origin"}),
		firstPeriodRequested:      *promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "gosk_transfer_first_period_requested", Help: "first period data was requested for this cycle, partitioned by origin"}, []string{"origin"}),
		lastPeriodRequested:       *promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "gosk_transfer_last_period_requested", Help: "last period data was requested for this cycle, partitioned by origin"}, []string{"origin"}),
	}

	if result.numberOfRequestWorkers == 0 {
		result.numberOfRequestWorkers = 1
	}
	result.dataRequestChannel = make(chan database.IncompletePeriod, result.numberOfRequestWorkers)

	return result
}

func (t *TransferRequester) Run() {
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

	var wg sync.WaitGroup
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

	var wg sync.WaitGroup
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
				t.countRequestsSent.With(prometheus.Labels{"origin": origin}).Inc()
			}
			wg.Done()
		}(origin, start)
	}

	wg.Wait()
}

func (t *TransferRequester) countResponseReceived(origin string, response ResponseMessage) {
	t.db.CreateRemoteCount(response.PeriodStart, origin, response.DataPoints)
	t.db.LogTransferRequest(origin, response)
	t.countResponsesReceived.With(prometheus.Labels{"origin": origin}).Inc()
}

func (t *TransferRequester) sendDataRequests() {
	incompletePeriods, err := t.db.SelectIncompletePeriods(t.completenessFactor)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve incomplete periods per origin",
			zap.String("Error", err.Error()),
			zap.Time("NextRequestAt", time.Now().Add(t.sleepBetweenDataRequests)),
		)

		time.Sleep(t.sleepBetweenDataRequests)
		return
	}

	incompletePeriodsGrouped := map[string][]database.IncompletePeriod{}
	for _, i := range incompletePeriods {
		if _, ok := incompletePeriodsGrouped[i.Origin]; !ok {
			incompletePeriodsGrouped[i.Origin] = make([]database.IncompletePeriod, 0)
		}
		incompletePeriodsGrouped[i.Origin] = append(incompletePeriodsGrouped[i.Origin], i)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		time.Sleep(t.sleepBetweenDataRequests)
		wg.Done()
	}()
	for origin, incompletePeriods := range incompletePeriodsGrouped {
		go func(incompletePeriods []database.IncompletePeriod) {
			t.dataMissingPeriods.With(prometheus.Labels{"origin": origin}).Set(float64(len(incompletePeriods)))
			var i int
			for i = range incompletePeriods {
				if i > t.maxPeriodsToRequest {
					t.firstPeriodRequested.With(prometheus.Labels{"origin": origin}).Set(float64(incompletePeriods[i].Period.Unix()))
					break
				}
				t.dataRequestChannel <- incompletePeriods[i]
			}
			t.firstPeriodRequested.With(prometheus.Labels{"origin": origin}).Set(float64(incompletePeriods[i].Period.Unix()))
			t.lastPeriodRequested.With(prometheus.Labels{"origin": origin}).Set(float64(incompletePeriods[0].Period.Unix()))
		}(incompletePeriods)
	}
	wg.Wait()
}

func (t *TransferRequester) sendDataRequestWorker(dataRequests <-chan database.IncompletePeriod) {
	for request := range dataRequests {
		countsPerUuid, err := t.db.SelectCountPerUuid(request.Origin, request.Period)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not retrieve counts per uuid from database",
				zap.String("Error", err.Error()),
				zap.String("Origin", request.Origin),
				zap.Time("Start", request.Period),
			)
			continue
		}

		logger.GetLogger().Info(
			"Sending data request for",
			zap.String("Origin", request.Origin),
			zap.Time("Period", request.Period),
			zap.Int("Local count", request.LocalCount),
			zap.Int("Remote count", request.RemoteCount),
			zap.Float64("Completeness", float64(request.LocalCount)/float64(request.RemoteCount)),
		)
		requestMessage := RequestMessage{
			Command:       dataCmd,
			UUID:          uuid.New(),
			PeriodStart:   request.Period,
			CountsPerUuid: countsPerUuid,
		}
		t.sendMQTTCommand(request.Origin, requestMessage)
		t.db.LogTransferRequest(request.Origin, requestMessage)
		t.dataRequestsSent.With(prometheus.Labels{"origin": request.Origin}).Inc()
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
