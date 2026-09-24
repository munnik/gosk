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

// OutboxReceiver is the far end of the scheme in outbox.go: it stores what
// a vessel ships and then, and only then, says so.
//
// The ordering is the whole point. The existing ingest path
// (reader/mqtt.go into writer/postgresql) hands rows to a batch that
// flushes later, so a process that acknowledged on receipt would be
// promising durability it does not have - a restart between the two loses
// the rows and the vessel has already been told to forget them. This
// writes synchronously and acknowledges afterwards, which is slower per
// shipment and is the only version that is actually safe.
type OutboxReceiver struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	codec      *outboxCodec

	// One shipment at a time per receiver. The write has to complete
	// before the acknowledgement, and paho calls this handler
	// concurrently (SetOrderMatters(false)).
	writeMutex sync.Mutex

	received prometheus.Counter
	stored   prometheus.Counter
	acked    prometheus.Counter
	failed   prometheus.Counter
}

func NewOutboxReceiver(c *config.TransferConfig) *OutboxReceiver {
	return &OutboxReceiver{
		db:       database.NewPostgresqlDatabase(&c.PostgresqlConfig),
		config:   c,
		codec:    newOutboxCodec(c.MQTTConfig.Compress),
		received: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_received_total", Help: "total number of outbox shipments received"}),
		stored:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_stored_total", Help: "total number of mapped rows stored from outbox shipments"}),
		acked:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_acks_sent_total", Help: "total number of outbox acknowledgements sent"}),
		failed:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_write_failures_total", Help: "total number of outbox shipments that could not be stored"}),
	}
}

func (r *OutboxReceiver) Run() {
	// Every origin: this is the cloud side, listening for whichever
	// vessels ship.
	r.mqttClient = mqtt.New(&r.config.MQTTConfig, "outboxReceive", r.shipmentReceived, fmt.Sprintf(outboxDataTopic, "#"))
	defer r.mqttClient.Disconnect()

	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait() // never exit
}

func (r *OutboxReceiver) shipmentReceived(c paho.Client, m paho.Message) {
	var shipment OutboxMessage
	if err := r.codec.decode(m.Payload(), &shipment); err != nil {
		logger.GetLogger().Warn(
			"Could not decode an outbox shipment",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}
	r.received.Inc()

	r.writeMutex.Lock()
	defer r.writeMutex.Unlock()

	for i := range shipment.Deltas {
		r.db.WriteMapped(&shipment.Deltas[i])
	}

	// Force the batch out and wait for it. Without this the rows are
	// merely queued, and acknowledging here would be a promise the
	// process cannot keep across a restart.
	if err := r.db.Flush(); err != nil {
		r.failed.Inc()
		logger.GetLogger().Warn(
			"Could not store an outbox shipment, not acknowledging it",
			zap.String("Error", err.Error()),
			zap.String("Shipment", shipment.Shipment.String()),
			zap.String("Origin", shipment.Origin),
		)
		// No acknowledgement. The vessel keeps the entries and sends
		// them again, which is exactly what should happen.
		return
	}

	rows := 0
	for _, d := range shipment.Deltas {
		rows += len(d.Updates)
	}
	r.stored.Add(float64(rows))

	// Acknowledge every uuid the shipment named, including any that
	// carried no rows: those are source messages whose data has aged out
	// at the far end, and leaving them unacknowledged would keep the
	// vessel sending an empty shipment forever.
	ack := OutboxAck{
		Shipment: shipment.Shipment,
		Origin:   shipment.Origin,
		Uuids:    shipment.Uuids,
		Stored:   rows,
	}
	payload, err := r.codec.encode(ack)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not encode an outbox acknowledgement",
			zap.String("Error", err.Error()),
		)
		return
	}

	r.mqttClient.Publish(fmt.Sprintf(outboxAckTopic, shipment.Origin), transferQoS, transferRetained, payload)
	r.acked.Inc()
	logger.GetLogger().Info(
		"Stored and acknowledged an outbox shipment",
		zap.String("Shipment", shipment.Shipment.String()),
		zap.String("Origin", shipment.Origin),
		zap.Int("Uuids", len(shipment.Uuids)),
		zap.Int("Rows", rows),
	)
}
