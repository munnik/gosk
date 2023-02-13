package writer

import (
	"encoding/json"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type PostgresqlWriter struct {
	db                   *database.PostgresqlDatabase
	mappedChannel        chan message.Mapped
	rawChannel           chan message.Raw
	numberOfWorkers      int
	messagesReceived     prometheus.Counter
	messagesUnmarshalled prometheus.Counter
	messagesWritten      prometheus.Counter
}

func NewPostgresqlWriter(c *config.PostgresqlConfig) *PostgresqlWriter {
	return &PostgresqlWriter{
		db:                   database.NewPostgresqlDatabase(c),
		mappedChannel:        make(chan message.Mapped, c.BufferSize),
		rawChannel:           make(chan message.Raw, c.BufferSize),
		numberOfWorkers:      c.NumberOfWorkers,
		messagesReceived:     promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_messages_received_total", Help: "total number of received nano messages"}),
		messagesUnmarshalled: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_messages_unmarshalled_total", Help: "total number of unmarshalled nano messages"}),
		messagesWritten:      promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_messages_written_total", Help: "total number of nano messages sent to db"}),
	}
}

func (w *PostgresqlWriter) StartRawWorkers() {
	var wg sync.WaitGroup
	wg.Add(w.numberOfWorkers)
	for i := 0; i < w.numberOfWorkers; i++ {
		go func() {
			for raw := range w.rawChannel {
				w.db.WriteRaw(raw)
				w.messagesWritten.Inc()
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func (w *PostgresqlWriter) WriteRaw(subscriber mangos.Socket) {
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.messagesReceived.Inc()
		raw := message.Raw{}
		if err := json.Unmarshal(received, &raw); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.messagesUnmarshalled.Inc()
		w.rawChannel <- raw
	}
}

func (w *PostgresqlWriter) StartMappedWorkers() {
	var wg sync.WaitGroup
	wg.Add(w.numberOfWorkers)
	for i := 0; i < w.numberOfWorkers; i++ {
		go func() {
			for mapped := range w.mappedChannel {
				w.db.WriteMapped(mapped)
				w.messagesWritten.Inc()
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func (w *PostgresqlWriter) WriteMapped(subscriber mangos.Socket) {
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.messagesReceived.Inc()
		mapped := message.Mapped{}
		if err := json.Unmarshal(received, &mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.messagesUnmarshalled.Inc()
		w.mappedChannel <- mapped
	}
}
