package transfer

import (
	"errors"
	"fmt"
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
)

// pruneEvery is how often the sender drops fully acknowledged buckets
// from transfer_outbox, and pruneOlderThan how long after a bucket closed
// that happens. Two hours covers a sensor whose clock is a little behind
// without costing a whole bucket for each late row it sends.
const (
	pruneEvery     = time.Minute
	pruneOlderThan = 2 * time.Hour
)

// Sender is the vessel half of the outbox: it sends the rows of pending
// buckets to the far end and records the far end's acknowledgements.
type Sender struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	codec      *codec

	// inFlight remembers when each bucket was last sent, and under which
	// generation, so a delivery the far end never acknowledged is retried
	// after RetryAfter rather than on every pass, and the pass does not
	// re-send what is still on its way. One entry per bucket: a bucket
	// reset by a late row simply replaces its entry, so nothing lingers
	// for generations that are gone.
	inFlight      map[inFlightKey]inFlightEntry
	inFlightMutex sync.Mutex
	// connected is what sendOnce last saw of the broker link, so that it
	// logs the transitions and not every cycle of an outage.
	connected bool

	sent     prometheus.Counter
	rowsSent prometheus.Counter
	acked    prometheus.Counter
	stale    prometheus.Counter
	retried  prometheus.Counter
	depth    *prometheus.GaugeVec
}

type inFlightKey struct {
	origin string
	bucket int64
}

type inFlightEntry struct {
	generation int64
	sent       time.Time
}

func NewSender(c *config.TransferConfig) *Sender {
	return &Sender{
		db:       database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:   c,
		codec:    newCodec(c.MQTTConfig.Compress),
		inFlight: make(map[inFlightKey]inFlightEntry),
		// Assumed up until seen down, so the first cycle of a process
		// that starts without its broker logs the outage once.
		connected: true,
		sent:      promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_deliveries_total", Help: "total number of outbox deliveries sent"}),
		rowsSent:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_rows_sent_total", Help: "total number of mapped rows sent through the outbox"}),
		acked:     promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_acked_total", Help: "total number of outbox acknowledgements that applied"}),
		stale:     promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_stale_acks_total", Help: "total number of outbox acknowledgements for a bucket generation that no longer existed"}),
		retried:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_retried_total", Help: "total number of buckets sent again because no acknowledgement arrived in time"}),
		depth:     promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "gosk_outbox_depth", Help: "buckets waiting to be acknowledged, partitioned by origin"}, []string{"origin"}),
	}
}

func (s *Sender) Run() {
	s.mqttClient = mqtt.New(&s.config.MQTTConfig, "transferSend", s.ackReceived, fmt.Sprintf(ackTopic, s.config.Origin))
	s.mqttClient.Connect()
	defer s.mqttClient.Disconnect()

	lastPrune := time.Now()
	for {
		s.sendOnce()
		if time.Since(lastPrune) > pruneEvery {
			if n, err := s.db.PruneOutbox(pruneOlderThan); err != nil {
				logger.GetLogger().Warn("Could not prune the outbox", zap.Error(err))
			} else if n > 0 {
				logger.GetLogger().Info("Pruned acknowledged buckets from the outbox", zap.Int64("Buckets", n))
			}
			lastPrune = time.Now()
		}
		time.Sleep(s.config.Interval)
	}
}

// sendOnce sends one delivery for each pending bucket that is not already
// on its way: the newest first, then the oldest, up to MaxBucketsPerCycle.
func (s *Sender) sendOnce() {
	entries, err := s.db.SelectPendingOutbox(s.config.MaxBucketsPerCycle)
	if err != nil {
		logger.GetLogger().Warn("Could not read the outbox", zap.Error(err))
		return
	}

	if depth, err := s.db.OutboxDepth(); err == nil {
		s.depth.Reset()
		for origin, n := range depth {
			s.depth.With(prometheus.Labels{"origin": origin}).Set(float64(n))
		}
	}

	if !s.mqttClient.Connected() {
		// Nothing is handed to paho while the link is down. It would
		// take it - paho queues a QoS 1 publish in memory while it
		// reconnects - and once the entry's retry interval passes this
		// would hand it the same bucket again, and again, for as long as
		// the outage lasts: a week offline with a seeded backlog would
		// be hundreds of copies of each bucket waiting in memory for the
		// moment the link comes back. The rows are safe in the outbox
		// until then; that is what it is for.
		if s.connected {
			logger.GetLogger().Warn("Not connected to the MQTT broker, holding the outbox until it is")
			s.connected = false
		}
		return
	}
	if !s.connected {
		logger.GetLogger().Info("Connected to the MQTT broker, sending from the outbox again")
		s.connected = true
	}

	for _, entry := range entries {
		if s.isInFlight(entry) {
			continue
		}
		s.send(entry)
	}
}

func (s *Sender) send(entry database.OutboxEntry) {
	rows, through, err := s.db.ReadOutboxDelivery(entry, s.config.MaxRowsPerDelivery)
	if errors.Is(err, database.ErrOutboxEntryGone) {
		// Reset by a late row or acknowledged meanwhile; the next pass
		// sees whatever it has become.
		return
	}
	if err != nil {
		logger.GetLogger().Warn(
			"Could not read the rows for an outbox delivery",
			zap.Error(err),
			zap.String("Origin", entry.Origin),
			zap.Time("Bucket", entry.Bucket),
		)
		return
	}

	delivery := Delivery{
		Delivery:   uuid.New(),
		Origin:     entry.Origin,
		Bucket:     entry.Bucket,
		Generation: entry.Generation,
		Through:    through,
		Deltas:     groupRows(rows),
	}
	payload, err := s.codec.encode(delivery)
	if err != nil {
		logger.GetLogger().Warn("Could not encode an outbox delivery", zap.Error(err))
		return
	}

	// Marked before the publish, so that an acknowledgement arriving
	// faster than this goroutine resumes still finds the entry.
	s.markInFlight(entry)
	s.mqttClient.Publish(fmt.Sprintf(dataTopic, entry.Origin), outboxQoS, outboxRetained, payload)

	s.sent.Inc()
	s.rowsSent.Add(float64(len(rows)))
	logger.GetLogger().Info(
		"Sent an outbox delivery",
		zap.String("Delivery", delivery.Delivery.String()),
		zap.String("Origin", entry.Origin),
		zap.Time("Bucket", entry.Bucket),
		zap.Int64("Generation", entry.Generation),
		zap.Time("Through", through),
		zap.Int("Rows", len(rows)),
		zap.Int("Bytes", len(payload)),
	)
}

func (s *Sender) ackReceived(c paho.Client, m paho.Message) {
	var ack Ack
	if err := s.codec.decode(m.Payload(), &ack); err != nil {
		logger.GetLogger().Warn(
			"Could not decode an outbox acknowledgement",
			zap.Error(err),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}

	// This is the only thing that moves a bucket forward, and it applies
	// only to the generation it was sent for: a bucket reset by a late row
	// in the meantime keeps its fresh generation and nothing acknowledged.
	applied, err := s.db.AckOutbox(ack.Origin, ack.Bucket, ack.Generation, ack.Through)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not record an outbox acknowledgement",
			zap.Error(err),
			zap.String("Origin", ack.Origin),
			zap.Time("Bucket", ack.Bucket),
		)
		return
	}
	if !applied {
		// Either the generation is gone (reset by a late row, or pruned)
		// or this is an old acknowledgement arriving again, behind a
		// newer one - QoS 1 does that. Neither says anything about the
		// delivery currently on its way, so the in-flight entry stays.
		s.stale.Inc()
		logger.GetLogger().Info(
			"An outbox acknowledgement that no longer applies was ignored",
			zap.String("Delivery", ack.Delivery.String()),
			zap.String("Origin", ack.Origin),
			zap.Time("Bucket", ack.Bucket),
			zap.Int64("Generation", ack.Generation),
		)
		return
	}
	s.clearInFlight(ack)
	s.acked.Inc()
	logger.GetLogger().Info(
		"An outbox delivery was acknowledged",
		zap.String("Delivery", ack.Delivery.String()),
		zap.String("Origin", ack.Origin),
		zap.Time("Bucket", ack.Bucket),
		zap.Time("Through", ack.Through),
		zap.Int("RowsStored", ack.Stored),
	)
}

func (s *Sender) isInFlight(entry database.OutboxEntry) bool {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()

	key := inFlightKey{origin: entry.Origin, bucket: entry.Bucket.UnixNano()}
	flight, ok := s.inFlight[key]
	if !ok {
		return false
	}
	if flight.generation != entry.Generation {
		// Reset since it was sent: whatever is on its way will be
		// acknowledged under the old generation and ignored.
		delete(s.inFlight, key)
		return false
	}
	if time.Since(flight.sent) < s.config.RetryAfter {
		return true
	}
	// Long enough without an acknowledgement that the far end is assumed
	// to have dropped it. Forget it was sent, so this pass picks it up.
	delete(s.inFlight, key)
	s.retried.Inc()
	return false
}

func (s *Sender) markInFlight(entry database.OutboxEntry) {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()
	s.inFlight[inFlightKey{origin: entry.Origin, bucket: entry.Bucket.UnixNano()}] = inFlightEntry{generation: entry.Generation, sent: time.Now()}
}

func (s *Sender) clearInFlight(ack Ack) {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()
	key := inFlightKey{origin: ack.Origin, bucket: ack.Bucket.UnixNano()}
	if flight, ok := s.inFlight[key]; ok && flight.generation == ack.Generation {
		delete(s.inFlight, key)
	}
}
