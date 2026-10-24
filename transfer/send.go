package transfer

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
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

	// epoch is this run's, reserved from the far end before anything is
	// sent (reserveEpoch); see 20261020120000_outbox_epochs. Zero until
	// then, and acknowledgements arriving meanwhile - queued for this
	// session by the broker - are not applied. Atomic because the MQTT
	// handlers read it while Run sets it.
	epoch atomic.Int64
	// epochReplies carries the far end's answers to this run's
	// reservation request, matched on reservation.
	epochReplies chan DigestReply
	reservation  uuid.UUID
	reserveMutex sync.Mutex

	sent     prometheus.Counter
	rowsSent prometheus.Counter
	// settledEmpty counts buckets settled with an empty delivery, see send.
	settledEmpty prometheus.Counter
	// oversized counts deliveries that could not be made to fit
	// maxDecodedPayload, see send.
	oversized prometheus.Counter
	acked     prometheus.Counter
	stale     prometheus.Counter
	retried   prometheus.Counter
	depth     *prometheus.GaugeVec
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
		// Buffered so a reply never waits on a reservation loop that has
		// moved on; extras are dropped.
		epochReplies: make(chan DigestReply, 4),
		// Assumed up until seen down, so the first cycle of a process
		// that starts without its broker logs the outage once.
		connected:    true,
		sent:         promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_deliveries_total", Help: "total number of outbox deliveries sent"}),
		rowsSent:     promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_rows_sent_total", Help: "total number of mapped rows sent through the outbox"}),
		settledEmpty: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_buckets_settled_empty_total", Help: "total number of pending outbox buckets settled with an empty delivery because their rows were no longer there"}),
		oversized:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_deliveries_oversized_total", Help: "total number of outbox deliveries that could not be made small enough for the far end to decode"}),
		acked:        promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_acked_total", Help: "total number of outbox acknowledgements that applied"}),
		stale:        promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_stale_acks_total", Help: "total number of outbox acknowledgements for a bucket generation that no longer existed"}),
		retried:      promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_retried_total", Help: "total number of buckets sent again because no acknowledgement arrived in time"}),
		depth:        promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "gosk_outbox_depth", Help: "buckets waiting to be acknowledged, partitioned by origin"}, []string{"origin"}),
	}
}

func (s *Sender) Run() {
	s.mqttClient = mqtt.New(&s.config.MQTTConfig, "transferSend", s.messageReceived, fmt.Sprintf(ackTopic, s.config.Origin), fmt.Sprintf(digestReplyTopic, s.config.Origin))
	s.mqttClient.Connect()
	defer s.mqttClient.Disconnect()

	s.epoch.Store(s.reserveEpoch())

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
	// Read, and encode, at most MaxRowsPerDelivery rows; if that does not
	// fit what the far end will decode (maxDecodedPayload), read again with
	// half as many. Each read records how far it reached as sent_through,
	// which only ever moves forward and only feeds the late-row check, so
	// reading the same bucket again with a smaller limit is harmless. A
	// delivery the far end can never decode would otherwise be sent, and
	// refused, forever.
	var (
		rows    []message.SingleValueMapped
		through time.Time
		payload []byte
		plain   int
	)
	delivery := Delivery{Origin: entry.Origin, Bucket: entry.Bucket, Generation: entry.Generation, Epoch: s.epoch.Load()}
	for limit := s.config.MaxRowsPerDelivery; ; limit /= 2 {
		var err error
		rows, through, err = s.db.ReadOutboxDelivery(entry, limit, delivery.Epoch)
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
		delivery.Delivery = uuid.New()
		delivery.Through = through
		delivery.Deltas = groupRows(rows)
		payload, plain, err = s.codec.encodeSized(delivery)
		if err != nil {
			logger.GetLogger().Warn("Could not encode an outbox delivery", zap.Error(err))
			return
		}
		if plain <= maxDecodedPayload {
			break
		}
		if limit <= 1 || len(rows) <= 1 {
			// Rows that share one timestamp go in one delivery, so
			// there is nothing left to split.
			s.oversized.Inc()
			logger.GetLogger().Error(
				"An outbox delivery cannot be made small enough for the far end to accept; the bucket stays pending",
				zap.String("Origin", entry.Origin),
				zap.Time("Bucket", entry.Bucket),
				zap.Int("Rows", len(rows)),
				zap.Int("Bytes", plain),
			)
			// Held back as if it had been sent, so it is tried again
			// after RetryAfter rather than re-read and re-encoded on
			// every pass. It needs over 64 MiB of rows at one
			// microsecond from one origin, which nothing on this fleet
			// produces.
			s.markInFlight(entry)
			return
		}
	}

	if len(rows) == 0 {
		// The bucket is still pending - ReadOutboxDelivery checked under
		// its lock - but none of its rows after AckedThrough are in
		// mapped_data any more. Every path that sets a bucket's latest
		// does so from rows committed alongside it, so they were deleted
		// afterwards: the disk-pressure trim drops whole chunks. There is
		// nothing left to send, and the empty delivery settles the bucket
		// rather than leave it pending forever; what it must not do is
		// look like an ordinary delivery. Whether the far end has those
		// rows is not known here - a delivery of them may have been
		// stored and its acknowledgement lost - so the warning says what
		// is certain: they were never acknowledged.
		s.settledEmpty.Inc()
		logger.GetLogger().Warn(
			"Settling an outbox bucket whose unacknowledged rows were deleted; unless the far end stored them before its acknowledgement was lost, they are lost",
			zap.String("Origin", entry.Origin),
			zap.Time("Bucket", entry.Bucket),
			zap.Time("AckedThrough", entry.AckedThrough),
			zap.Time("Latest", entry.Latest),
		)
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

	epoch := s.epoch.Load()
	if !ackOfRun(ack, epoch) {
		// From an earlier run of this sender. It can name a generation
		// this run still holds and a through beyond what this run has
		// sent - a restored database keeps a pending bucket's generation
		// - so applying it could settle rows that were never delivered.
		// The bucket goes again under this run's epoch.
		s.stale.Inc()
		logger.GetLogger().Info(
			"An outbox acknowledgement from an earlier run of this sender was ignored",
			zap.String("Delivery", ack.Delivery.String()),
			zap.String("Origin", ack.Origin),
			zap.Time("Bucket", ack.Bucket),
			zap.Int64("Epoch", ack.Epoch),
			zap.Int64("ThisRun", epoch),
		)
		return
	}

	// This is the only thing that moves a bucket forward, and it applies
	// only to the generation it was sent for: a bucket reset by a late row
	// in the meantime keeps its fresh generation and nothing acknowledged.
	applied, err := s.db.AckOutbox(ack.Origin, ack.Bucket, epoch, ack.Generation, ack.Through)
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

// ackOfRun reports whether an acknowledgement belongs to the run whose epoch
// is epoch. One without an epoch is from before this sender knew epochs, or
// from a far end older than them - which this sender cannot be talking to,
// since it reserves its epoch from that far end before it sends - and is
// not this run's either. Nor is anything before the reservation is done.
func ackOfRun(ack Ack, epoch int64) bool {
	return epoch != 0 && ack.Epoch == epoch
}

// messageReceived hands each message to what its topic is for: the
// acknowledgements, and the answer to this run's epoch reservation.
func (s *Sender) messageReceived(c paho.Client, m paho.Message) {
	if m.Topic() == fmt.Sprintf(digestReplyTopic, s.config.Origin) {
		s.epochReplyReceived(m)
		return
	}
	s.ackReceived(c, m)
}

func (s *Sender) epochReplyReceived(m paho.Message) {
	var reply DigestReply
	if err := s.codec.decode(m.Payload(), &reply); err != nil {
		return
	}
	s.reserveMutex.Lock()
	ours := reply.Exchange == s.reservation && reply.Epoch > 0
	s.reserveMutex.Unlock()
	if !ours {
		// The seeder's replies arrive here too, and it takes them.
		return
	}
	select {
	case s.epochReplies <- reply:
	default:
	}
}

// reserveEpoch asks the far end for this run's epoch and waits until it has
// one, asking again after SeedTimeout as often as it takes. Nothing is sent
// until then: a delivery under the previous run's epoch - or none - is what
// a restored database must not send. A far end that is unreachable, or older
// than epochs and silent, therefore holds the outbox back; receivers are
// upgraded before senders.
func (s *Sender) reserveEpoch() int64 {
	for {
		last, err := s.db.LastOutboxEpoch()
		if err != nil {
			logger.GetLogger().Warn("Could not read the last outbox epoch this sender reserved; trying again", zap.Error(err))
			time.Sleep(s.config.RetryAfter)
			continue
		}
		exchange := uuid.New()
		s.reserveMutex.Lock()
		s.reservation = exchange
		s.reserveMutex.Unlock()

		payload, err := s.codec.encode(DigestOffer{Exchange: exchange, Origin: s.config.Origin, ReserveEpoch: true, Epoch: last})
		if err != nil {
			logger.GetLogger().Fatal("Could not encode a sender epoch reservation", zap.Error(err))
		}
		// The answer comes back on the digest-reply topic, and is not
		// retained: ask only once this session is subscribed to it.
		if !s.mqttClient.WaitSubscribed(s.config.SeedTimeout) {
			logger.GetLogger().Warn("Not subscribed to the epoch reservation's reply yet; waiting", zap.String("Origin", s.config.Origin))
			continue
		}
		s.mqttClient.Publish(fmt.Sprintf(digestTopic, s.config.Origin), outboxQoS, outboxRetained, payload)

		timeout := time.After(s.config.SeedTimeout)
		for {
			select {
			case reply := <-s.epochReplies:
				if reply.Exchange != exchange {
					continue
				}
				if reply.Epoch <= last {
					// A far end that ignored the floor; ask again rather
					// than reuse an epoch.
					break
				}
				if err := s.db.RecordOutboxEpoch(reply.Epoch); err != nil {
					// Without the record a far end remade from an older
					// copy could hand the next run this number again.
					logger.GetLogger().Warn("Could not record this run's outbox epoch; reserving again", zap.Error(err))
					break
				}
				logger.GetLogger().Info("Reserved this run's outbox epoch", zap.String("Origin", s.config.Origin), zap.Int64("Epoch", reply.Epoch))
				return reply.Epoch
			case <-timeout:
				logger.GetLogger().Warn(
					"The far end did not answer this run's epoch reservation; asking again, and sending nothing until it does",
					zap.String("Origin", s.config.Origin),
					zap.Duration("After", s.config.SeedTimeout),
				)
			}
			break
		}
	}
}
