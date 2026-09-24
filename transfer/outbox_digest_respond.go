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

// OutboxDigestResponder is the cloud half of the digest walk in
// outbox_digest.go. It answers a vessel's summaries with what it can and
// cannot account for.
//
// It is a separate process from OutboxReceiver rather than another
// subscription on it, because the two do different work at different
// rates: receiving is a write that must complete before it is
// acknowledged, and answering a digest is a long read over an index. One
// of them blocking the other would mean either a stalled seeding run or,
// worse, shipments not being stored while a summary is computed.
//
// It never writes. A vessel decides what to enqueue; this only reports.
type OutboxDigestResponder struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	codec      *outboxCodec

	// Summaries are expensive reads, and a fleet seeding at once would
	// otherwise put as many of them on the database as there are vessels.
	// One at a time per responder; run more processes to answer faster.
	answerMutex sync.Mutex

	offersReceived  prometheus.Counter
	bucketsCompared prometheus.Counter
	bucketsDiffered prometheus.Counter
	uuidsCompared   prometheus.Counter
	uuidsMissing    prometheus.Counter
}

func NewOutboxDigestResponder(c *config.TransferConfig) *OutboxDigestResponder {
	return &OutboxDigestResponder{
		db:              database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:          c,
		codec:           newOutboxCodec(c.MQTTConfig.Compress),
		offersReceived:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_offers_received_total", Help: "total number of digest offers received from vessels"}),
		bucketsCompared: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_buckets_compared_total", Help: "total number of history buckets compared"}),
		bucketsDiffered: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_buckets_differed_total", Help: "total number of history buckets that did not match and had to be opened up"}),
		uuidsCompared:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_uuids_compared_total", Help: "total number of source messages compared one by one"}),
		uuidsMissing:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_digest_uuids_missing_total", Help: "total number of source messages found to be missing here"}),
	}
}

func (r *OutboxDigestResponder) Run() {
	r.mqttClient = mqtt.New(&r.config.MQTTConfig, "outboxDigest", r.offerReceived, fmt.Sprintf(outboxDigestTopic, "#"))
	defer r.mqttClient.Disconnect()

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait() // never exit
}

func (r *OutboxDigestResponder) offerReceived(c paho.Client, m paho.Message) {
	var offer OutboxDigestOffer
	if err := r.codec.decode(m.Payload(), &offer); err != nil {
		logger.GetLogger().Warn(
			"Could not decode a digest offer",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	r.offersReceived.Inc()

	r.answerMutex.Lock()
	defer r.answerMutex.Unlock()

	reply, err := r.answer(offer)
	if err != nil {
		// No reply at all. The vessel retries and then skips the bucket,
		// which leaves it uncompared rather than wrongly declared equal -
		// an answer invented here would be one that told a vessel its
		// history was accounted for when nobody had looked.
		logger.GetLogger().Warn(
			"Could not answer a digest offer",
			zap.String("Error", err.Error()),
			zap.String("Origin", offer.Origin),
			zap.String("Prefix", offer.Prefix),
		)
		return
	}

	payload, err := r.codec.encode(reply)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not encode a digest reply",
			zap.String("Error", err.Error()),
		)
		return
	}
	r.mqttClient.Publish(fmt.Sprintf(outboxDigestReplyTopic, offer.Origin), transferQoS, transferRetained, payload)
}

func (r *OutboxDigestResponder) answer(offer OutboxDigestOffer) (OutboxDigestReply, error) {
	lo, hi, err := prefixBounds(offer.Prefix)
	if err != nil {
		return OutboxDigestReply{}, err
	}
	reply := OutboxDigestReply{Exchange: offer.Exchange, Origin: offer.Origin}

	switch {
	case len(offer.Leaves) > 0:
		mine, err := r.db.SelectUuidsInPrefix(offer.Origin, lo, hi)
		if err != nil {
			return OutboxDigestReply{}, err
		}
		reply.Missing = compareLeaves(offer.Leaves, mine)
		reply.Complete = len(reply.Missing) == 0
		r.uuidsCompared.Add(float64(len(offer.Leaves)))
		r.uuidsMissing.Add(float64(len(reply.Missing)))
		logger.GetLogger().Info(
			"Compared a bucket of source messages",
			zap.String("Origin", offer.Origin),
			zap.String("Prefix", offer.Prefix),
			zap.Int("Offered", len(offer.Leaves)),
			zap.Int("Missing", len(reply.Missing)),
		)

	case len(offer.Buckets) > 0:
		// The same query over the same range at the same depth, which is
		// what makes the two summaries comparable. The depth is taken from
		// the offer rather than recomputed, because the vessel chooses how
		// wide to split based on how much it holds, and this end holds
		// something different by definition.
		mine, err := r.db.SelectUuidDigest(offer.Origin, lo, hi, len(offer.Buckets[0].Prefix))
		if err != nil {
			return OutboxDigestReply{}, err
		}
		reply.Descend = compareBuckets(offer.Buckets, mine)
		reply.Complete = len(reply.Descend) == 0
		r.bucketsCompared.Add(float64(len(offer.Buckets)))
		r.bucketsDiffered.Add(float64(len(reply.Descend)))
		logger.GetLogger().Info(
			"Compared a level of history",
			zap.String("Origin", offer.Origin),
			zap.String("Prefix", offer.Prefix),
			zap.Int("Offered", len(offer.Buckets)),
			zap.Int("Differ", len(reply.Descend)),
		)

	default:
		// An offer describing nothing. The vessel holds nothing under this
		// prefix, so there is nothing here to be missing.
		reply.Complete = true
	}

	return reply, nil
}
