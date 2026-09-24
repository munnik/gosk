package transfer

import (
	"fmt"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/uuid/v5"
	"go.uber.org/zap"
)

// OutboxSeeder is the vessel half of the digest walk in outbox_digest.go:
// it compares its history with the far end's and puts whatever the far end
// is missing into transfer_outbox, where the ordinary shipper picks it up.
//
// It seeds and exits. Nothing about it is a background loop: it answers
// "what does the cloud not have" once, and having answered it, its work is
// the outbox depth it leaves behind. Run it again later and it costs one
// pass over the summaries to find nothing, which is the cheap case by
// construction.
//
// It writes nothing but outbox entries. It does not send data, does not
// delete anything, and can be interrupted at any point - EnqueueOutbox is
// idempotent and the walk is deterministic, so a second run resumes by
// simply doing the same thing again.
type OutboxSeeder struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	codec      *outboxCodec

	// replies carries what the far end says back. Buffered so a reply that
	// arrives just after its exchange timed out does not block paho's
	// handler; such a reply is recognised by its exchange id and dropped.
	replies chan OutboxDigestReply

	// Counted for the report at the end. A seeding run is something an
	// operator watches, not a service that is scraped, so these are logged
	// rather than exported.
	exchanges  int
	offered    int64
	enqueued   int64
	timedOut   int
	deepest    int
	startedAt  time.Time
	lastReport time.Time
}

// seedBucket is a bucket waiting to be described, with the count this
// vessel already knows for it. The count comes from the parent's summary,
// which is why it does not have to be asked for again: it decides whether
// this bucket is opened up or named outright.
type seedBucket struct {
	prefix string
	uuids  int64
}

func NewOutboxSeeder(c *config.TransferConfig) *OutboxSeeder {
	return &OutboxSeeder{
		db:      database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:  c,
		codec:   newOutboxCodec(c.MQTTConfig.Compress),
		replies: make(chan OutboxDigestReply, 8),
	}
}

// Run walks the whole of this origin's history and returns once every
// bucket has been either agreed on or accounted for.
func (s *OutboxSeeder) Run() error {
	s.mqttClient = mqtt.New(&s.config.MQTTConfig, "outboxSeed", s.replyReceived, fmt.Sprintf(outboxDigestReplyTopic, s.config.Origin))
	defer s.mqttClient.Disconnect()

	s.startedAt = time.Now()
	s.lastReport = s.startedAt
	logger.GetLogger().Info(
		"Seeding the outbox from history",
		zap.String("Origin", s.config.Origin),
		zap.Int("LeafSize", s.config.OutboxSeedLeafSize),
	)

	// Breadth first, so the walk reports whole levels of history rather
	// than plunging down one branch - and so an operator watching the log
	// sees the shape of the problem early. It also keeps the work
	// spread over time rather than concentrated in whichever branch
	// happens to be first.
	queue := []seedBucket{{prefix: "", uuids: 0}}
	for len(queue) > 0 {
		bucket := queue[0]
		queue = queue[1:]

		children, err := s.walk(bucket)
		if err != nil {
			return err
		}
		queue = append(queue, children...)

		s.report(len(queue))
		time.Sleep(s.config.OutboxSeedPause)
	}

	logger.GetLogger().Info(
		"Finished seeding the outbox from history",
		zap.String("Origin", s.config.Origin),
		zap.Duration("Took", time.Since(s.startedAt)),
		zap.Int("Exchanges", s.exchanges),
		zap.Int64("SourceMessagesCompared", s.offered),
		zap.Int64("SourceMessagesEnqueued", s.enqueued),
		zap.Int("TimedOut", s.timedOut),
	)
	if s.timedOut > 0 {
		return fmt.Errorf("%d exchanges went unanswered, so parts of history were not compared; run this again", s.timedOut)
	}
	return nil
}

// walk describes one bucket to the far end and returns the buckets that
// have to be described in more detail.
func (s *OutboxSeeder) walk(bucket seedBucket) ([]seedBucket, error) {
	lo, hi, err := prefixBounds(bucket.prefix)
	if err != nil {
		return nil, err
	}
	if len(bucket.prefix) > s.deepest {
		s.deepest = len(bucket.prefix)
	}

	// Small enough to name outright. Never at the root, whose count is not
	// known until something has been summarised.
	if bucket.prefix != "" && bucket.uuids <= int64(s.config.OutboxSeedLeafSize) {
		return nil, s.walkLeaf(bucket, lo, hi)
	}

	buckets, err := s.db.SelectUuidDigest(s.config.Origin, lo, hi, splitLength(bucket.prefix, bucket.uuids, s.config.OutboxSeedLeafSize))
	if err != nil {
		return nil, fmt.Errorf("could not summarise %q: %w", bucket.prefix, err)
	}
	if len(buckets) == 0 {
		// Nothing here. Not an error: a parent bucket's count came from a
		// summary taken a moment ago, and retention may since have
		// removed what it counted.
		return nil, nil
	}

	reply, ok := s.exchange(OutboxDigestOffer{Origin: s.config.Origin, Prefix: bucket.prefix, Buckets: buckets})
	if !ok {
		return nil, nil
	}

	// The counts the children carry down come from this summary, so the
	// next level does not have to ask the database how big anything is.
	counts := make(map[string]int64, len(buckets))
	for _, b := range buckets {
		counts[b.Prefix] = b.Uuids
		s.offered += b.Uuids
	}

	children := make([]seedBucket, 0, len(reply.Descend))
	for _, prefix := range reply.Descend {
		children = append(children, seedBucket{prefix: prefix, uuids: counts[prefix]})
	}
	if len(children) > 0 {
		logger.GetLogger().Info(
			"Part of history differs",
			zap.String("Prefix", bucket.prefix),
			zap.Int("Offered", len(buckets)),
			zap.Int("Differ", len(children)),
		)
	}
	return children, nil
}

// walkLeaf names a bucket's source messages and enqueues whichever the far
// end says it lacks. This is the only place the seeder writes anything.
func (s *OutboxSeeder) walkLeaf(bucket seedBucket, lo, hi uuid.UUID) error {
	leaves, err := s.db.SelectUuidsInPrefix(s.config.Origin, lo, hi)
	if err != nil {
		return fmt.Errorf("could not list %q: %w", bucket.prefix, err)
	}
	if len(leaves) == 0 {
		return nil
	}
	s.offered += int64(len(leaves))

	reply, ok := s.exchange(OutboxDigestOffer{Origin: s.config.Origin, Prefix: bucket.prefix, Leaves: leaves})
	if !ok {
		return nil
	}
	if len(reply.Missing) == 0 {
		return nil
	}

	enqueued, err := s.db.EnqueueOutbox(s.config.Origin, reply.Missing)
	if err != nil {
		return fmt.Errorf("could not enqueue the missing source messages under %q: %w", bucket.prefix, err)
	}
	s.enqueued += enqueued
	logger.GetLogger().Info(
		"Enqueued source messages the far end is missing",
		zap.String("Prefix", bucket.prefix),
		zap.Int("Offered", len(leaves)),
		zap.Int("Missing", len(reply.Missing)),
		zap.Int64("Enqueued", enqueued),
	)
	return nil
}

// exchange publishes an offer and waits for its reply, retrying a few
// times before giving up on that bucket.
//
// Giving up is safe and is why the walk carries on rather than failing:
// a bucket that was never compared is a bucket whose contents were not
// enqueued, which is the state the vessel was already in. Run returns an
// error at the end if anything was skipped, so a run that did not manage
// to compare everything is not reported as a clean one.
func (s *OutboxSeeder) exchange(offer OutboxDigestOffer) (OutboxDigestReply, bool) {
	offer.Exchange = uuid.Must(uuid.NewV7Precise())
	payload, err := s.codec.encode(offer)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not encode a digest offer",
			zap.String("Error", err.Error()),
			zap.String("Prefix", offer.Prefix),
		)
		return OutboxDigestReply{}, false
	}

	for attempt := 1; attempt <= outboxSeedAttempts; attempt++ {
		s.mqttClient.Publish(fmt.Sprintf(outboxDigestTopic, offer.Origin), transferQoS, transferRetained, payload)
		s.exchanges++
		if reply, ok := s.await(offer.Exchange); ok {
			return reply, true
		}
		logger.GetLogger().Warn(
			"No answer to a digest offer",
			zap.String("Prefix", offer.Prefix),
			zap.Int("Attempt", attempt),
			zap.Int("Of", outboxSeedAttempts),
		)
	}

	s.timedOut++
	return OutboxDigestReply{}, false
}

// await waits for the reply to one exchange, discarding answers to
// exchanges that have already timed out.
func (s *OutboxSeeder) await(exchange uuid.UUID) (OutboxDigestReply, bool) {
	deadline := time.After(s.config.OutboxSeedTimeout)
	for {
		select {
		case reply := <-s.replies:
			if reply.Exchange == exchange {
				return reply, true
			}
			// A late answer to a question already asked again. Dropping it
			// is right: the same question is outstanding, and this answer
			// describes the same range either way.
		case <-deadline:
			return OutboxDigestReply{}, false
		}
	}
}

func (s *OutboxSeeder) replyReceived(c paho.Client, m paho.Message) {
	var reply OutboxDigestReply
	if err := s.codec.decode(m.Payload(), &reply); err != nil {
		logger.GetLogger().Warn(
			"Could not decode a digest reply",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	select {
	case s.replies <- reply:
	default:
		// Nothing is waiting and the buffer is full: only possible for
		// replies to exchanges that have long since timed out.
	}
}

// report logs progress at intervals, because a first run on a vessel with
// years of history is long enough that silence is indistinguishable from
// being stuck.
func (s *OutboxSeeder) report(queued int) {
	if time.Since(s.lastReport) < outboxSeedReportEvery {
		return
	}
	s.lastReport = time.Now()
	logger.GetLogger().Info(
		"Still seeding",
		zap.String("Origin", s.config.Origin),
		zap.Duration("Running", time.Since(s.startedAt)),
		zap.Int("BucketsQueued", queued),
		zap.Int("Exchanges", s.exchanges),
		zap.Int("DeepestPrefix", s.deepest),
		zap.Int64("SourceMessagesEnqueued", s.enqueued),
	)
}

const (
	// outboxSeedAttempts is how many times one offer is sent before the
	// bucket it describes is skipped. The offer goes at QoS 1, so this
	// covers the far end being down or too busy rather than the message
	// being lost.
	outboxSeedAttempts = 3
	// outboxSeedReportEvery is how often a long run says it is still going.
	outboxSeedReportEvery = 30 * time.Second
)
