package transfer

import (
	"fmt"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mqtt"
	"go.uber.org/zap"
)

// seedRetries is how many times one offer is sent before the ranges it
// covered are given up on for this run.
const seedRetries = 3

// Seeder is the vessel half of the walk in digest.go: it compares its
// history with the far end's and puts whatever the far end is missing into
// transfer_outbox, where the sender picks it up.
//
// It seeds and exits. It answers "what does the far end not have" once,
// and having answered it, its work is the outbox depth it leaves behind.
// Run it again later and it costs one pass over the summaries to find
// nothing, which is the cheap case by construction.
//
// It writes nothing but outbox entries. It does not send data, does not
// delete anything, and can be interrupted at any point - enqueueing is
// idempotent and the walk is deterministic, so a second run resumes by
// doing the same thing again.
type Seeder struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	codec      *codec

	// replies carries what the far end says back. Buffered so a reply that
	// arrives just after its exchange timed out does not block paho's
	// handler; such a reply is recognised by its exchange id and dropped.
	replies chan DigestReply

	// Counted for the report at the end. A seeding run is something an
	// operator watches, not a service that is scraped, so these are logged
	// rather than exported.
	exchanges int
	compared  int64
	enqueued  int64
	timedOut  int
	startedAt time.Time
}

func NewSeeder(c *config.TransferConfig) *Seeder {
	return &Seeder{
		db:      database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:  c,
		codec:   newCodec(c.MQTTConfig.Compress),
		replies: make(chan DigestReply, 8),
	}
}

// Run walks this origin's history between from and to and returns once
// every bucket has been either agreed on or put in the outbox. A zero from
// means the oldest row this vessel holds; a zero to means the start of the
// current minute, which the sender is already handling.
func (s *Seeder) Run(from, to time.Time) error {
	if from.IsZero() {
		earliest, ok, err := s.db.EarliestMapped(s.config.Origin)
		if err != nil {
			return fmt.Errorf("could not find the oldest row: %w", err)
		}
		if !ok {
			logger.GetLogger().Info("Nothing to seed, this database holds no rows for the origin", zap.String("Origin", s.config.Origin))
			return nil
		}
		from = earliest
	}
	if to.IsZero() {
		to = time.Now().Truncate(database.OutboxBucket)
	}
	if !to.After(from) {
		return fmt.Errorf("nothing to seed between %v and %v", from, to)
	}

	s.mqttClient = mqtt.New(&s.config.MQTTConfig, "transferSeed", s.replyReceived, fmt.Sprintf(digestReplyTopic, s.config.Origin))
	s.mqttClient.Connect()
	defer s.mqttClient.Disconnect()

	s.startedAt = time.Now()
	logger.GetLogger().Info(
		"Seeding the outbox from history",
		zap.String("Origin", s.config.Origin),
		zap.Time("From", from),
		zap.Time("To", to),
	)

	// One level at a time, over all the ranges that level has to look at:
	// the whole span at the top, then the buckets that differed at each
	// level below. Breadth first, so an operator watching the log sees the
	// shape of the problem early.
	width := digestLevels[0]
	ranges := []TimeRange{alignRange(TimeRange{From: from, To: to}, width)}
	for len(ranges) > 0 {
		next, err := s.walkLevel(width, ranges)
		if err != nil {
			return err
		}
		narrower, ok := nextLevel(width)
		if !ok {
			break
		}
		width, ranges = narrower, next
	}

	logger.GetLogger().Info(
		"Finished seeding the outbox from history",
		zap.String("Origin", s.config.Origin),
		zap.Duration("Took", time.Since(s.startedAt)),
		zap.Int("Exchanges", s.exchanges),
		zap.Int64("BucketsCompared", s.compared),
		zap.Int64("BucketsEnqueued", s.enqueued),
		zap.Int("TimedOut", s.timedOut),
	)
	if s.timedOut > 0 {
		return fmt.Errorf("%d exchanges went unanswered, so parts of history were not compared; run this again", s.timedOut)
	}
	return nil
}

// walkLevel summarises ranges at width, offers the summaries, enqueues
// what the far end is missing outright, and returns the ranges the next
// level has to open up. At the last level a bucket that differs is
// enqueued rather than returned.
func (s *Seeder) walkLevel(width time.Duration, ranges []TimeRange) ([]TimeRange, error) {
	buckets := []database.DigestBucket{}
	for _, r := range ranges {
		b, err := s.db.SelectDigest(s.config.Origin, r.From, r.To, width)
		if err != nil {
			return nil, fmt.Errorf("could not summarise %v to %v: %w", r.From, r.To, err)
		}
		buckets = append(buckets, b...)
	}
	s.compared += int64(len(buckets))

	_, hasNext := nextLevel(width)
	next := []TimeRange{}
	for _, offer := range chunkOffers(ranges, buckets, width, s.config.SeedMaxBuckets) {
		offer.Origin = s.config.Origin
		reply, ok := s.exchange(offer)
		if !ok {
			continue
		}
		for _, start := range reply.Missing {
			if err := s.enqueue(start, width); err != nil {
				return nil, err
			}
		}
		for _, start := range reply.Differ {
			if !hasNext {
				if err := s.enqueue(start, width); err != nil {
					return nil, err
				}
				continue
			}
			next = append(next, TimeRange{From: start, To: start.Add(width)})
		}
		if len(reply.Missing)+len(reply.Differ) > 0 {
			logger.GetLogger().Info(
				"Part of history differs",
				zap.Duration("Width", width),
				zap.Int("Offered", len(offer.Buckets)),
				zap.Int("Differ", len(reply.Differ)),
				zap.Int("Missing", len(reply.Missing)),
			)
		}
		time.Sleep(s.config.SeedPause)
	}
	return next, nil
}

// enqueue puts every outbox bucket between start and start+width into the
// outbox. This is the only place the seeder writes anything.
func (s *Seeder) enqueue(start time.Time, width time.Duration) error {
	n, err := s.db.EnqueueOutboxRange(s.config.Origin, start, start.Add(width))
	if err != nil {
		return fmt.Errorf("could not enqueue %v to %v: %w", start, start.Add(width), err)
	}
	s.enqueued += n
	return nil
}

// exchange publishes an offer and waits for its reply, retrying a few
// times before giving up on the ranges it covered.
//
// Giving up is safe and is why the walk carries on rather than failing: a
// range that was never compared is a range whose contents were not
// enqueued, which is the state the vessel was already in. Run returns an
// error at the end if anything was skipped, so a run that did not manage
// to compare everything is not reported as a clean one.
func (s *Seeder) exchange(offer DigestOffer) (DigestReply, bool) {
	for attempt := 1; attempt <= seedRetries; attempt++ {
		offer.Exchange = uuid.New()
		payload, err := s.codec.encode(offer)
		if err != nil {
			logger.GetLogger().Warn("Could not encode a digest offer", zap.Error(err))
			return DigestReply{}, false
		}
		s.exchanges++
		s.mqttClient.Publish(fmt.Sprintf(digestTopic, s.config.Origin), outboxQoS, outboxRetained, payload)

		deadline := time.NewTimer(s.config.SeedTimeout)
		for {
			select {
			case reply := <-s.replies:
				if reply.Exchange != offer.Exchange {
					// The answer to an offer that timed out and was
					// asked again; the fresh one is on its way.
					continue
				}
				deadline.Stop()
				return reply, true
			case <-deadline.C:
				logger.GetLogger().Warn(
					"A digest offer went unanswered",
					zap.Int("Attempt", attempt),
					zap.Duration("Width", offer.Width),
					zap.Int("Ranges", len(offer.Ranges)),
					zap.Int("Buckets", len(offer.Buckets)),
				)
			}
			break
		}
	}
	s.timedOut++
	return DigestReply{}, false
}

func (s *Seeder) replyReceived(c paho.Client, m paho.Message) {
	var reply DigestReply
	if err := s.codec.decode(m.Payload(), &reply); err != nil {
		logger.GetLogger().Warn("Could not decode a digest reply", zap.Error(err), zap.ByteString("Bytes", m.Payload()))
		return
	}
	select {
	case s.replies <- reply:
	default:
		// Nobody waiting and the buffer full: a stale answer.
	}
}
