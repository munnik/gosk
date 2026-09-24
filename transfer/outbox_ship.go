package transfer

import (
	"fmt"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/mqtt"
	"github.com/munnik/uuid/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// OutboxShipper is the vessel half of the scheme in outbox.go: it works
// through transfer_outbox oldest first, sends each batch to the far end,
// and removes entries only once the far end says it has stored them.
type OutboxShipper struct {
	db         *database.PostgresqlDatabase
	config     *config.TransferConfig
	mqttClient *mqtt.Client
	codec      *outboxCodec

	// inFlight remembers when each uuid was last sent, so a shipment the
	// far end never acknowledged is retried rather than blocking the
	// queue behind it. Bounded by the outbox batch size times the number
	// of shipments that can be outstanding at once.
	inFlight      map[uuid.UUID]time.Time
	inFlightMutex sync.Mutex

	shipped  prometheus.Counter
	acked    prometheus.Counter
	retried  prometheus.Counter
	depth    prometheus.GaugeVec
	rowsSent prometheus.Counter
}

func NewOutboxShipper(c *config.TransferConfig) *OutboxShipper {
	db := database.NewPostgresqlDatabase(&c.PostgresqlConfig)
	return &OutboxShipper{
		db:       db,
		config:   c,
		codec:    newOutboxCodec(c.MQTTConfig.Compress),
		inFlight: make(map[uuid.UUID]time.Time),
		shipped:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_shipments_total", Help: "total number of outbox shipments sent"}),
		acked:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_acked_total", Help: "total number of source messages acknowledged by the far end"}),
		retried:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_retried_total", Help: "total number of source messages sent again because no acknowledgement arrived"}),
		depth:    *promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "gosk_outbox_depth", Help: "source messages waiting to be acknowledged, partitioned by origin"}, []string{"origin"}),
		rowsSent: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_outbox_rows_sent_total", Help: "total number of mapped rows sent through the outbox"}),
	}
}

func (s *OutboxShipper) Run() {
	s.mqttClient = mqtt.New(&s.config.MQTTConfig, "outboxShip", s.ackReceived, fmt.Sprintf(outboxAckTopic, s.config.Origin))
	defer s.mqttClient.Disconnect()

	for {
		if shipped := s.shipOnce(); shipped == 0 {
			// Nothing to do, or nothing that is not already in flight.
			// Either way there is no point spinning.
			time.Sleep(s.config.OutboxIdleInterval)
		}
	}
}

// shipOnce sends at most one batch and returns how many source messages it
// covered.
func (s *OutboxShipper) shipOnce() int {
	// Over-read: entries already in flight are filtered out below, and
	// without the margin a batch's worth of in-flight work would hide
	// everything behind it.
	entries, err := s.db.SelectOutbox(s.config.OutboxBatchSize * 4)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not read the outbox",
			zap.String("Error", err.Error()),
		)
		time.Sleep(s.config.OutboxIdleInterval)
		return 0
	}

	if depth, err := s.db.OutboxDepth(); err == nil {
		for origin, n := range depth {
			s.depth.With(prometheus.Labels{"origin": origin}).Set(float64(n))
		}
	}

	// One origin per shipment: the far end acknowledges per origin, and a
	// vessel's own outbox is almost always a single origin anyway.
	byOrigin := make(map[string][]uuid.UUID)
	for _, e := range entries {
		if s.isInFlight(e.Uuid) {
			continue
		}
		if len(byOrigin[e.Origin]) >= s.config.OutboxBatchSize {
			continue
		}
		byOrigin[e.Origin] = append(byOrigin[e.Origin], e.Uuid)
	}

	total := 0
	for origin, uuids := range byOrigin {
		if s.ship(origin, uuids) {
			total += len(uuids)
		}
	}
	return total
}

func (s *OutboxShipper) ship(origin string, uuids []uuid.UUID) bool {
	deltas, err := s.db.SelectMappedByUuids(origin, uuids)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not read the rows for an outbox shipment",
			zap.String("Error", err.Error()),
			zap.String("Origin", origin),
			zap.Int("Uuids", len(uuids)),
		)
		return false
	}

	// Sent by value: OutboxMessage is marshalled once and the pointers
	// would otherwise outlive the read.
	values := make([]message.Mapped, 0, len(deltas))
	for _, d := range deltas {
		values = append(values, *d)
	}

	shipment := OutboxMessage{
		Shipment: uuid.Must(uuid.NewV7Precise()),
		Origin:   origin,
		Uuids:    uuids,
		Deltas:   values,
	}

	payload, err := s.codec.encode(shipment)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not encode an outbox shipment",
			zap.String("Error", err.Error()),
		)
		return false
	}

	s.mqttClient.Publish(fmt.Sprintf(outboxDataTopic, origin), transferQoS, transferRetained, payload)
	s.markInFlight(uuids)

	s.shipped.Inc()
	s.rowsSent.Add(float64(len(values)))
	logger.GetLogger().Info(
		"Sent an outbox shipment",
		zap.String("Shipment", shipment.Shipment.String()),
		zap.String("Origin", origin),
		zap.Int("Uuids", len(uuids)),
		zap.Int("Rows", len(values)),
		zap.Int("Bytes", len(payload)),
	)
	return true
}

func (s *OutboxShipper) ackReceived(c paho.Client, m paho.Message) {
	var ack OutboxAck
	if err := s.codec.decode(m.Payload(), &ack); err != nil {
		logger.GetLogger().Warn(
			"Could not decode an outbox acknowledgement",
			zap.String("Error", err.Error()),
			zap.ByteString("Bytes", m.Payload()),
		)
		return
	}

	// This is the only place anything leaves the outbox, and it happens
	// only for uuids the far end has named. A shipment that was sent and
	// never acknowledged stays pending and is sent again; there is no
	// cursor that could have moved past it in the meantime.
	removed, err := s.db.DeleteOutbox(ack.Origin, ack.Uuids)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not clear acknowledged entries from the outbox",
			zap.String("Error", err.Error()),
			zap.String("Origin", ack.Origin),
		)
		return
	}

	s.clearInFlight(ack.Uuids)
	s.acked.Add(float64(removed))
	logger.GetLogger().Info(
		"An outbox shipment was acknowledged",
		zap.String("Shipment", ack.Shipment.String()),
		zap.String("Origin", ack.Origin),
		zap.Int64("Cleared", removed),
		zap.Int("RowsStored", ack.Stored),
	)
}

func (s *OutboxShipper) isInFlight(u uuid.UUID) bool {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()

	sent, ok := s.inFlight[u]
	if !ok {
		return false
	}
	if time.Since(sent) < outboxRetryAfter {
		return true
	}
	// Long enough without an acknowledgement that the far end is assumed
	// to have dropped it. Forget it was sent, so the next pass picks it up.
	delete(s.inFlight, u)
	s.retried.Inc()
	return false
}

func (s *OutboxShipper) markInFlight(uuids []uuid.UUID) {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()
	now := time.Now()
	for _, u := range uuids {
		s.inFlight[u] = now
	}
}

func (s *OutboxShipper) clearInFlight(uuids []uuid.UUID) {
	s.inFlightMutex.Lock()
	defer s.inFlightMutex.Unlock()
	for _, u := range uuids {
		delete(s.inFlight, u)
	}
}
