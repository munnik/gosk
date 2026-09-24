package transfer

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/uuid/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// randomDuration returns a duration in [0, d). rand.Int64N panics on a
// non-positive argument, which a zero or negative sleep interval in the
// configuration would otherwise reach.
func randomDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(d)))
}

type TransferRequester struct {
	db                        *database.PostgresqlDatabase
	mqttConfig                *config.MQTTConfig
	mqttClient                *mqtt.Client
	sleepBetweenCountRequests time.Duration
	sleepBetweenDataRequests  time.Duration
	numberOfRequestWorkers    int
	maxPeriodsToRequest       int
	maxCountRequestsPerCycle  int
	completenessFactor        float64
	dataRequestChannel        chan database.IncompletePeriod
	countRequestsSent         prometheus.CounterVec
	countResponsesReceived    prometheus.CounterVec
	dataRequestsSent          prometheus.CounterVec
	countMissingPeriods       prometheus.GaugeVec
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
		maxCountRequestsPerCycle:  c.MaxCountRequestsPerCycle,
		completenessFactor:        c.CompletenessFactor,
		countRequestsSent:         *promauto.NewCounterVec(prometheus.CounterOpts{Name: "gosk_transfer_count_requests_total", Help: "total number of count requests sent, partitioned by origin"}, []string{"origin"}),
		countResponsesReceived:    *promauto.NewCounterVec(prometheus.CounterOpts{Name: "gosk_transfer_count_responses_total", Help: "total number of count responses received, partitioned by origin"}, []string{"origin"}),
		dataRequestsSent:          *promauto.NewCounterVec(prometheus.CounterOpts{Name: "gosk_transfer_data_requests_total", Help: "total number of data requests sent, partitioned by origin"}, []string{"origin"}),
		countMissingPeriods:       *promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "gosk_transfer_missing_counts_total", Help: "total number of periods without a remote count, partitioned by origin"}, []string{"origin"}),
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
	t.mqttClient = mqtt.New(t.mqttConfig, "transferRequest", t.messageReceived, fmt.Sprintf(respondTopic, "#"))
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
			defer wg.Done()

			// wait random amount of time before processing to spread the workload
			time.Sleep(randomDuration(t.sleepBetweenCountRequests))

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

			// An origin only gets a remote count for a period once it has
			// answered a request about it, so everything it did not answer
			// - every period it was unreachable for - is still in this
			// list on the next cycle, and the list only grows. Asking
			// about all of it every cycle means a vessel that was offline
			// for a week costs ~2000 publishes and ~2000 log rows per
			// cycle, for as long as the gap stays unfilled. Ask about the
			// newest ones and let the rest wait: a genuine backlog still
			// drains, a cycle at a time, while a gap that opened an hour
			// ago is still closed at the first opportunity.
			t.countMissingPeriods.With(prometheus.Labels{"origin": origin}).Set(float64(len(periods)))
			if t.maxCountRequestsPerCycle > 0 && len(periods) > t.maxCountRequestsPerCycle {
				logger.GetLogger().Info(
					"More periods without a remote count than one cycle sends, requesting the newest ones",
					zap.String("Origin", origin),
					zap.Int("Periods", len(periods)),
					zap.Int("Requesting", t.maxCountRequestsPerCycle),
				)
				periods = periods[len(periods)-t.maxCountRequestsPerCycle:]
			}

			for _, period := range periods {
				requestMessage := RequestMessage{
					Command:     countCmd,
					UUID:        uuid.Must(uuid.NewV7Precise()),
					PeriodStart: period,
				}
				t.sendMQTTCommand(origin, requestMessage)
				t.db.LogTransferRequest(origin, requestMessage)
				t.countRequestsSent.With(prometheus.Labels{"origin": origin}).Inc()
			}
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

	// Every one of these goroutines has to be waited on, not just the
	// sleeper. They feed dataRequestChannel, which holds
	// numberOfRequestWorkers entries and is drained by that many workers -
	// so with more incomplete periods than workers can get through in
	// sleepBetweenDataRequests (the normal case: the default is 500
	// periods against 5 workers, each doing a query, a publish and a log
	// write) they are still blocked on the send when the sleep ends.
	// Returning there let the caller's `for { t.sendDataRequests() }` start
	// a second full set of them for the same origins, and a third, each
	// queueing the same requests again behind the last - goroutines and
	// duplicate requests accumulating for as long as the backlog lasted.
	var wg sync.WaitGroup
	wg.Add(len(incompletePeriodsGrouped) + 1)

	go func() {
		defer wg.Done()
		time.Sleep(t.sleepBetweenDataRequests)
	}()
	for origin, incompletePeriods := range incompletePeriodsGrouped {
		go func(origin string, incompletePeriods []database.IncompletePeriod) {
			defer wg.Done()

			t.dataMissingPeriods.With(prometheus.Labels{"origin": origin}).Set(float64(len(incompletePeriods)))

			// Ordered newest period first, see selectIncompletePeriodsQuery.
			last := len(incompletePeriods) - 1
			if t.maxPeriodsToRequest > 0 && last >= t.maxPeriodsToRequest {
				last = t.maxPeriodsToRequest - 1
			}
			for i := 0; i <= last; i++ {
				t.dataRequestChannel <- incompletePeriods[i]
			}

			t.firstPeriodRequested.With(prometheus.Labels{"origin": origin}).Set(float64(incompletePeriods[last].Period.Unix()))
			t.lastPeriodRequested.With(prometheus.Labels{"origin": origin}).Set(float64(incompletePeriods[0].Period.Unix()))
		}(origin, incompletePeriods)
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
			UUID:          uuid.Must(uuid.NewV7Precise()),
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
	t.mqttClient.Publish(topic, transferQoS, transferRetained, bytes)
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
