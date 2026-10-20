package transfer

import (
	"fmt"
	"sync"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mqtt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// DigestResponder is the far end of the walk in digest.go. It answers a
// vessel's summaries with what it can and cannot account for.
//
// It is a separate process from Receiver rather than another subscription
// on it, because the two do different work at different rates: receiving
// is a write that must complete before it is acknowledged, and answering
// a digest is a long read over a range of chunks. One blocking the other
// would mean either a stalled seeding run or deliveries not being stored
// while a summary is computed.
//
// It never writes. A vessel decides what to enqueue; this only reports.
type DigestResponder struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	codec      *codec

	// Summaries are expensive reads, and a fleet seeding at once would
	// otherwise put as many of them on the database as there are vessels.
	// One at a time per responder; run more processes to answer faster.
	answerMutex sync.Mutex

	offersReceived  prometheus.Counter
	bucketsCompared prometheus.Counter
	bucketsDiffered prometheus.Counter
	bucketsMissing  prometheus.Counter
}

func NewDigestResponder(c *config.TransferConfig) *DigestResponder {
	return &DigestResponder{
		db:              database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:          c,
		codec:           newCodec(c.MQTTConfig.Compress),
		offersReceived:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_offers_received_total", Help: "total number of digest offers received from vessels"}),
		bucketsCompared: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_buckets_compared_total", Help: "total number of history buckets compared"}),
		bucketsDiffered: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_buckets_differed_total", Help: "total number of history buckets that did not match and were opened up"}),
		bucketsMissing:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_buckets_missing_total", Help: "total number of history buckets so empty here that the whole bucket was asked for"}),
	}
}

func (r *DigestResponder) Run() {
	r.mqttClient = mqtt.New(&r.config.MQTTConfig, "transferDigest", r.offerReceived, fmt.Sprintf(digestTopic, "#"))
	r.mqttClient.Connect()
	defer r.mqttClient.Disconnect()

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait() // never exit
}

func (r *DigestResponder) offerReceived(c paho.Client, m paho.Message) {
	var offer DigestOffer
	if err := r.codec.decode(m.Payload(), &offer); err != nil {
		logger.GetLogger().Warn(
			"Could not decode a digest offer",
			zap.Error(err),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	r.offersReceived.Inc()
	if err := validateOffer(m.Topic(), &offer); err != nil {
		logger.GetLogger().Warn(
			"Ignoring a digest offer that does not match its topic",
			zap.Error(err),
			zap.String("Topic", m.Topic()),
		)
		return
	}

	r.answerMutex.Lock()
	defer r.answerMutex.Unlock()

	reply, err := r.answer(offer)
	if err != nil {
		// No reply at all. The vessel retries and then skips the range,
		// which leaves it uncompared rather than wrongly declared equal -
		// an answer invented here would tell a vessel its history was
		// accounted for when nobody had looked.
		logger.GetLogger().Warn(
			"Could not answer a digest offer",
			zap.Error(err),
			zap.String("Origin", offer.Origin),
			zap.Duration("Width", offer.Width),
		)
		return
	}

	payload, err := r.codec.encode(reply)
	if err != nil {
		logger.GetLogger().Warn("Could not encode a digest reply", zap.Error(err))
		return
	}
	r.mqttClient.Publish(fmt.Sprintf(digestReplyTopic, offer.Origin), outboxQoS, outboxRetained, payload)
}

func (r *DigestResponder) answer(offer DigestOffer) (DigestReply, error) {
	if offer.Width <= 0 {
		return DigestReply{}, fmt.Errorf("offer with a width of %v", offer.Width)
	}

	// The same query over the same ranges at the same width, which is
	// what makes the two summaries comparable.
	mine := []database.DigestBucket{}
	for _, span := range offer.Ranges {
		buckets, err := r.db.SelectDigest(offer.Origin, span.From, span.To, offer.Width)
		if err != nil {
			return DigestReply{}, err
		}
		mine = append(mine, buckets...)
	}

	missing, differ := compareDigests(offer.Buckets, mine, r.config.SeedTakeWholeBelow)
	r.bucketsCompared.Add(float64(len(offer.Buckets)))
	r.bucketsDiffered.Add(float64(len(differ)))
	r.bucketsMissing.Add(float64(len(missing)))
	logger.GetLogger().Info(
		"Compared a level of history",
		zap.String("Origin", offer.Origin),
		zap.Duration("Width", offer.Width),
		zap.Int("Ranges", len(offer.Ranges)),
		zap.Int("Offered", len(offer.Buckets)),
		zap.Int("Differ", len(differ)),
		zap.Int("Missing", len(missing)),
	)

	return DigestReply{Exchange: offer.Exchange, Origin: offer.Origin, Missing: missing, Differ: differ}, nil
}
