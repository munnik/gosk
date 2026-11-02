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
	// confirmationPoll is how often awaitConfirmation looks whether the
	// broker has confirmed the delivery.
	confirmationPoll = 50 * time.Millisecond
)

// Sender is the vessel half of the outbox: it sends the rows of pending
// buckets to the far end and records the far end's acknowledgements.
type Sender struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient senderClient
	codec      *codec

	// inFlight remembers what was last sent for each bucket, and under
	// which generation, so a delivery the far end never acknowledged is
	// retried after RetryAfter rather than on every pass, and the pass
	// does not re-send what is still on its way - see isInFlight for what
	// that means. One entry per bucket: a bucket reset by a late row
	// simply replaces its entry, so nothing lingers for generations that
	// are gone.
	inFlight      map[inFlightKey]inFlightEntry
	inFlightMutex sync.Mutex
	// connected is what sendOnce last saw of the broker link, so that it
	// logs the transitions and not every cycle of an outage.
	connected bool
	// accepted is the fingerprint of the last delivery the MQTT client
	// took for sending on its connection, and whether there has been one;
	// while the client still holds it nothing else is handed over, see
	// awaitConfirmation. Run's goroutine alone touches these.
	accepted    uint64
	hasAccepted bool

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
	// unconfirmed is how many deliveries the MQTT client holds that the
	// broker has not confirmed, see isInFlight.
	unconfirmed prometheus.Gauge
}

// senderClient is what the sender asks of its MQTT client (mqtt.Client);
// named so a test can stand in for it.
type senderClient interface {
	Connect()
	Disconnect()
	Connected() bool
	WaitSubscribed(timeout time.Duration) bool
	Publish(topic string, qos byte, retained bool, payload []byte) paho.Token
	Holds(fingerprint uint64) bool
	Unconfirmed() int
	Stalled(d time.Duration) bool
	DropConnection()
}

type inFlightKey struct {
	origin string
	bucket int64
}

type inFlightEntry struct {
	generation int64
	sent       time.Time
	// published is whether a delivery was handed to the MQTT client,
	// fingerprint identifies it to the client (mqtt.Fingerprint) and
	// through is the newest row time it carries, which an acknowledgement
	// has to reach to be its (clearInFlight). A bucket held back without
	// a delivery (oversized, see send) has none of these.
	published   bool
	fingerprint uint64
	through     time.Time
	// confirmed is when the broker was first seen to have confirmed the
	// delivery - the first pass that found the client no longer holding
	// it - and from when RetryAfter counts; zero until then.
	confirmed time.Time
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
		unconfirmed:  promauto.NewGauge(prometheus.GaugeOpts{Name: "gosk_outbox_unconfirmed", Help: "outbox deliveries handed to the MQTT client that the broker has not confirmed; the client sends them again on every reconnect, and their buckets are not sent again until it has"}),
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
	s.unconfirmed.Set(float64(s.mqttClient.Unconfirmed()))

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

	if s.mqttClient.Stalled(2 * s.config.RetryAfter) {
		// The client has held a delivery unconfirmed, on this connection,
		// for twice RetryAfter. Whatever the reason - a write that makes
		// no progress, a delivery it stored because its writer did not
		// take it in time (notSent), one it stored as the link was going
		// - it sends what it holds on a new connection and on a new
		// connection only, and its keepalive is not sure to bring one
		// about: the ping goes out through the same stalled socket. So
		// the connection is dropped here, and the client re-sends on the
		// new one everything it holds. Every delivery the client holds
		// counts, whether or not a bucket still waits on it - a late row
		// resets a bucket and leaves its old delivery in the client's
		// hands - so nothing it holds escapes this. The rows are safe in
		// the outbox either way; this is the threshold past which the
		// connection is given up on, not a bound on how long a bucket
		// waits. Twice RetryAfter rather than once: the stall is judged
		// by time without a confirmation, not by the absence of
		// progress, and a delivery can take minutes to transmit on a
		// poor link - dropping the connection under it throws away what
		// it had transmitted, so this errs towards waiting.
		s.mqttClient.DropConnection()
		return
	}

	for _, entry := range entries {
		if s.isInFlight(entry) {
			continue
		}
		if s.hasAccepted && s.mqttClient.Holds(s.accepted) {
			// One delivery in the client's hands at a time (see
			// awaitConfirmation), and the last one it took is still
			// there: the broker did not confirm it within its wait.
			// Nothing more this pass; the next one looks again.
			return
		}
		if !s.send(entry) {
			// The client will not send on this connection (notSent);
			// nothing more this pass.
			return
		}
	}
}

// send sends one delivery for a pending bucket, and reports whether the
// pass may go on to the next: false once the MQTT client will not send on
// this connection.
func (s *Sender) send(entry database.OutboxEntry) bool {
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
			return true
		}
		if err != nil {
			logger.GetLogger().Warn(
				"Could not read the rows for an outbox delivery",
				zap.Error(err),
				zap.String("Origin", entry.Origin),
				zap.Time("Bucket", entry.Bucket),
			)
			return true
		}
		delivery.Delivery = uuid.New()
		delivery.Through = through
		delivery.Deltas = groupRows(rows)
		payload, plain, err = s.codec.encodeSized(delivery)
		if err != nil {
			logger.GetLogger().Warn("Could not encode an outbox delivery", zap.Error(err))
			return true
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
			return true
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
	s.markPublished(entry, payload, through)
	token := s.mqttClient.Publish(fmt.Sprintf(dataTopic, entry.Origin), outboxQoS, outboxRetained, payload)
	fingerprint := mqtt.Fingerprint(payload)
	if err := publishError(token); err != nil {
		s.notSent(entry, fingerprint, err)
		return false
	}
	s.accepted, s.hasAccepted = fingerprint, true

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
	s.awaitConfirmation(token, fingerprint)
	return true
}

// publishError is the error a publish token already carries when paho
// will not send the message on this connection (mqtt.Client.Publish), nil
// while the token is still open or when it completed without one.
func publishError(token paho.Token) error {
	select {
	case <-token.Done():
		return token.Error()
	default:
		return nil
	}
}

// notSent handles a delivery paho will not send on this connection, which
// is one of two things. Not connected - the connection went between
// sendOnce's check and the publish - and paho stored nothing: the bucket is
// forgotten, not sent so not in flight, and the next pass that finds the
// link up sends it. Or paho's writer did not take the delivery within
// thirty seconds and paho stored it to send on its next connection: the
// bucket stays in flight behind it, held (isInFlight), and waits for that
// connection, while the pass ends (sendOnce) rather than hand a stalled
// writer more. That is paho's own path for it - resumed, confirmed, its
// message id freed - and taking the delivery out of paho's store instead
// would leave its id claimed for the life of the process. The wait is
// bounded: the link dropping brings the new connection, and if it does not
// drop, sendOnce drops it after twice RetryAfter (mqtt.Client.Stalled).
func (s *Sender) notSent(entry database.OutboxEntry, fingerprint uint64, err error) {
	if s.mqttClient.Holds(fingerprint) {
		logger.GetLogger().Warn(
			"The MQTT client stored an outbox delivery to send on its next connection; the bucket waits for that",
			zap.Error(err),
			zap.String("Origin", entry.Origin),
			zap.Time("Bucket", entry.Bucket),
		)
		return
	}
	s.forgetInFlight(entry)
	logger.GetLogger().Warn(
		"The MQTT client took no outbox delivery; the bucket will be sent again",
		zap.Error(err),
		zap.String("Origin", entry.Origin),
		zap.Time("Bucket", entry.Bucket),
	)
}

// awaitConfirmation waits, up to Interval, for the broker to confirm the
// delivery just handed to the MQTT client, so that the next delivery is
// handed to a client with nothing in its hands.
//
// One delivery at a time, because of how paho takes them: its writer
// sends one packet at a time, and a Publish behind a packet that is still
// being written waits for the writer, thirty seconds at most, after which
// paho reports "publish was broken by timeout" and leaves the packet in its
// store - unsent, and sent only on the next connection (notSent has the
// rest). Handing paho the next delivery only once the last is confirmed
// means the writer has nothing but paho's own control packets in hand when
// it is handed over. It is also what paces
// the sender to the link: a delivery takes what it takes to transmit and
// confirm, and the next one follows; a vessel with a good link clears a
// pass in a second or two, a vessel with a poor one sends what the link
// carries and nothing more. What it costs is the pipelining: a pass of
// eleven deliveries is eleven round trips to the broker, where it was one.
// Past Interval the pass ends and the next finds the client still holding
// the delivery (sendOnce) and sends nothing until it is confirmed.
//
// The token completes on the confirmation - or when paho re-queues the
// publish on a new connection, which is not one - so the store has the
// last word.
func (s *Sender) awaitConfirmation(token paho.Token, fingerprint uint64) {
	until := time.Now().Add(s.config.Interval)
	select {
	case <-token.Done():
	case <-time.After(time.Until(until)):
	}
	for s.mqttClient.Holds(fingerprint) && time.Now().Before(until) {
		time.Sleep(confirmationPoll)
	}
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

// isInFlight reports whether a pending bucket's last delivery is still on
// its way, so that the pass leaves the bucket alone; where it is not, the
// entry is forgotten so that the pass sends the bucket again.
//
// On its way means two things in turn. First, until the broker has
// confirmed the delivery, the MQTT client holds it and sends it again on
// every connection until the broker does (mqtt.Client.Holds): that may be
// seconds on a good link or hours on a vessel whose link is down, and in
// all that time there is nothing to add - another copy would wait in the
// same queue behind the first, and keep waiting there after the first was
// acknowledged, to be sent anyway and cost the link for nothing. On a
// link that drops every few minutes that is how an outbox stopped
// draining: every reconnect spent itself re-sending copies of buckets the
// far end had long acknowledged. Second, once the broker has it, for
// RetryAfter: the far end receives it, stores it and acknowledges, and
// only its silence for that long says the delivery or its acknowledgement
// was lost between broker and far end, or the far end could not store it.
//
// A delivery the client took nothing of is never in flight: send sees it
// on the publish token and forgets it at once. One the client stored to
// send on its next connection is in flight, held, until the broker
// confirms it there (notSent); sendOnce bounds how long that takes
// (mqtt.Client.Stalled).
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
	since := flight.sent
	if flight.published {
		if s.mqttClient.Holds(flight.fingerprint) {
			return true
		}
		if flight.confirmed.IsZero() {
			flight.confirmed = time.Now()
			s.inFlight[key] = flight
		}
		since = flight.confirmed
	}
	if time.Since(since) < s.config.RetryAfter {
		return true
	}
	// Long enough without an acknowledgement that the far end is assumed
	// to have dropped it. Forget it was sent, so this pass picks it up.
	delete(s.inFlight, key)
	s.retried.Inc()
	return false
}

// markInFlight holds a bucket back as if sent, without a delivery to wait
// for; see isInFlight.
func (s *Sender) markInFlight(entry database.OutboxEntry) {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()
	s.inFlight[inFlightKey{origin: entry.Origin, bucket: entry.Bucket.UnixNano()}] = inFlightEntry{generation: entry.Generation, sent: time.Now()}
}

// markPublished holds a bucket back behind the delivery about to be handed
// to the MQTT client, with this payload and reaching through; see
// isInFlight.
func (s *Sender) markPublished(entry database.OutboxEntry, payload []byte, through time.Time) {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()
	s.inFlight[inFlightKey{origin: entry.Origin, bucket: entry.Bucket.UnixNano()}] = inFlightEntry{
		generation:  entry.Generation,
		sent:        time.Now(),
		published:   true,
		fingerprint: mqtt.Fingerprint(payload),
		through:     through,
	}
}

// forgetInFlight drops a bucket's entry, if it is still the one made for
// this generation.
func (s *Sender) forgetInFlight(entry database.OutboxEntry) {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()
	key := inFlightKey{origin: entry.Origin, bucket: entry.Bucket.UnixNano()}
	if flight, ok := s.inFlight[key]; ok && flight.generation == entry.Generation {
		delete(s.inFlight, key)
	}
}

// clearInFlight settles a bucket's entry on an acknowledgement that
// applied, if the acknowledgement is of the delivery the entry is for or
// of a later one: it has to reach the entry's through. An earlier
// delivery's acknowledgement, arriving late behind the delivery that
// replaced it, says nothing about that one, which the client may well
// still hold.
func (s *Sender) clearInFlight(ack Ack) {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()
	key := inFlightKey{origin: ack.Origin, bucket: ack.Bucket.UnixNano()}
	if flight, ok := s.inFlight[key]; ok && flight.generation == ack.Generation && !ack.Through.Before(flight.through) {
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
