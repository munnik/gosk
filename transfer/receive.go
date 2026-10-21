package transfer

import (
	"fmt"
	"sync"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/gosk/nanomsg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// Receiver is the far end of the outbox: it stores what a vessel sends and
// then, and only then, says so.
//
// The ordering is the whole point. The existing ingest path (reader/mqtt.go
// into writer/postgresql) hands rows to a batch that flushes later, so a
// process that acknowledged on receipt would be promising durability it
// does not have - a restart between the two loses the rows and the vessel
// has already been told to forget them. This writes synchronously, in one
// transaction per delivery, and acknowledges afterwards.
type Receiver struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	codec      *codec

	// publish, when set, also hands what was stored to this process's
	// nanomsg subscribers, so whatever consumes live data on the far end
	// sees it the way it sees the reader's.
	publish chan *message.Mapped

	// One delivery at a time per receiver: paho calls the handler
	// concurrently (SetOrderMatters(false)) and the write has to complete
	// before the acknowledgement.
	writeMutex sync.Mutex
	received   prometheus.Counter
	stale      prometheus.Counter
	rejected   prometheus.Counter
	stored     prometheus.Counter
	acked      prometheus.Counter
	failed     prometheus.Counter
}

func NewReceiver(c *config.TransferConfig) *Receiver {
	return &Receiver{
		db:       database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:   c,
		codec:    newCodec(c.MQTTConfig.Compress),
		received: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_received_total", Help: "total number of outbox deliveries received"}),
		stale:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_stale_deliveries_total", Help: "total number of outbox deliveries not stored because a newer generation of their bucket already was"}),
		rejected: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_rejected_deliveries_total", Help: "total number of outbox deliveries not stored because they did not match their topic or bucket"}),
		stored:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_stored_total", Help: "total number of mapped rows stored from outbox deliveries"}),
		acked:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_acks_sent_total", Help: "total number of outbox acknowledgements sent"}),
		failed:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_write_failures_total", Help: "total number of outbox deliveries that could not be stored"}),
	}
}

// Run receives until the process ends. publisher may be nil, in which
// case what is stored is not re-published on nanomsg.
func (r *Receiver) Run(publisher *nanomsg.Publisher[message.Mapped]) {
	if publisher != nil {
		r.publish = make(chan *message.Mapped, 5000)
		defer close(r.publish)
		go publisher.Send(r.publish)
	}

	// Every origin: this is the far end, listening for whichever vessels
	// send.
	r.mqttClient = mqtt.New(&r.config.MQTTConfig, "transferReceive", r.deliveryReceived, fmt.Sprintf(dataTopic, "#"))
	r.mqttClient.Connect()
	defer r.mqttClient.Disconnect()

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait() // never exit
}

func (r *Receiver) deliveryReceived(c paho.Client, m paho.Message) {
	var delivery Delivery
	if err := r.codec.decode(m.Payload(), &delivery); err != nil {
		logger.GetLogger().Warn(
			"Could not decode an outbox delivery",
			zap.Error(err),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	r.received.Inc()
	if err := validateDelivery(m.Topic(), &delivery); err != nil {
		// Not stored and not acknowledged. A genuine sender never produces
		// one of these, so there is nothing to retry towards; the counter
		// is what makes a vessel sending them visible.
		r.rejected.Inc()
		logger.GetLogger().Warn(
			"Rejecting an outbox delivery that does not match its topic or bucket",
			zap.Error(err),
			zap.String("Topic", m.Topic()),
			zap.String("Delivery", delivery.Delivery.String()),
		)
		return
	}

	r.writeMutex.Lock()
	defer r.writeMutex.Unlock()

	deltas := make([]*message.Mapped, 0, len(delivery.Deltas))
	rows := 0
	for i := range delivery.Deltas {
		deltas = append(deltas, &delivery.Deltas[i])
		for _, u := range delivery.Deltas[i].Updates {
			rows += len(u.Values)
		}
	}

	stored, err := r.db.StoreDelivery(delivery.Origin, delivery.Bucket, delivery.Epoch, delivery.Generation, deltas)
	if err != nil {
		r.failed.Inc()
		logger.GetLogger().Warn(
			"Could not store an outbox delivery, not acknowledging it",
			zap.Error(err),
			zap.String("Delivery", delivery.Delivery.String()),
			zap.String("Origin", delivery.Origin),
			zap.Time("Bucket", delivery.Bucket),
		)
		// No acknowledgement. The vessel sends the bucket again after
		// its retry interval, which is exactly what should happen.
		return
	}
	if !stored {
		// A delivery of a generation this bucket has moved past: a late
		// row on the vessel reset the bucket, the whole of it was sent
		// again under the new generation and stored here, and now the
		// old delivery turns up after all (paho hands deliveries to this
		// handler concurrently, and the broker redelivers what was not
		// acknowledged at the time). Storing it would put whatever the
		// old delivery had for a row over what the new one had, and the
		// vessel, which is done with this generation, would never send
		// the new one again. Or a delivery from an earlier run of the
		// vessel's sender, which a later run's has replaced (see
		// 20261020120000_outbox_epochs). Not acknowledged either: the
		// vessel ignores an acknowledgement of a generation, or a run, it
		// no longer has.
		r.stale.Inc()
		logger.GetLogger().Info(
			"Not storing an outbox delivery that something newer has replaced",
			zap.String("Delivery", delivery.Delivery.String()),
			zap.String("Origin", delivery.Origin),
			zap.Time("Bucket", delivery.Bucket),
			zap.Int64("Epoch", delivery.Epoch),
			zap.Int64("Generation", delivery.Generation),
		)
		return
	}
	r.stored.Add(float64(rows))

	if r.publish != nil {
		for _, d := range deltas {
			select {
			case r.publish <- d:
			default:
				// A subscriber that cannot keep up does not hold the
				// acknowledgement back; the rows are committed.
			}
		}
	}

	if !r.config.Acknowledge {
		// A copy, not the keeper of the data: storing it (and passing it
		// on) is this end's whole job, and saying so would be a lie the
		// vessel acts on.
		return
	}

	ack := Ack{
		Delivery:   delivery.Delivery,
		Origin:     delivery.Origin,
		Bucket:     delivery.Bucket,
		Generation: delivery.Generation,
		Through:    delivery.Through,
		Stored:     rows,
		Epoch:      delivery.Epoch,
	}
	if !r.publishAck(ack) {
		return
	}
	r.acked.Inc()
	logger.GetLogger().Info(
		"Stored and acknowledged an outbox delivery",
		zap.String("Delivery", delivery.Delivery.String()),
		zap.String("Origin", delivery.Origin),
		zap.Time("Bucket", delivery.Bucket),
		zap.Time("Through", delivery.Through),
		zap.Int("Rows", rows),
	)
}

// publishAck sends an acknowledgement, or a refusal, to the vessel it is
// for, and reports whether it could be encoded.
func (r *Receiver) publishAck(ack Ack) bool {
	payload, err := r.codec.encode(ack)
	if err != nil {
		logger.GetLogger().Warn("Could not encode an outbox acknowledgement", zap.Error(err))
		return false
	}
	r.mqttClient.Publish(fmt.Sprintf(ackTopic, ack.Origin), outboxQoS, outboxRetained, payload)
	return true
}
