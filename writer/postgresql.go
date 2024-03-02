package writer

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type PostgresqlWriter struct {
	db                   *database.PostgresqlDatabase
	mappedChannel        chan *message.Mapped
	rawChannel           chan *message.Raw
	numberOfWorkers      int
	messagesReceived     prometheus.Counter
	messagesUnmarshalled prometheus.Counter
	messagesWritten      prometheus.Counter
}

func NewPostgresqlWriter(c *config.PostgresqlConfig) *PostgresqlWriter {
	// todo: 	defer close(mappedChannel)
	// todo: 	defer close(rawChannel)
	return &PostgresqlWriter{
		db:                   database.NewPostgresqlDatabase(c),
		mappedChannel:        make(chan *message.Mapped, c.BufferSize),
		rawChannel:           make(chan *message.Raw, c.BufferSize),
		numberOfWorkers:      c.NumberOfWorkers,
		messagesReceived:     promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_messages_received_total", Help: "total number of received nano messages"}),
		messagesUnmarshalled: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_messages_unmarshalled_total", Help: "total number of unmarshalled nano messages"}),
		messagesWritten:      promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_messages_written_total", Help: "total number of nano messages sent to db"}),
	}
}

func (w *PostgresqlWriter) StartRawWorkers() {
	wg := new(sync.WaitGroup)
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

func (w *PostgresqlWriter) WriteRaw(subscriber *nanomsg.Subscriber[message.Raw]) {
	receiveBuffer := make(chan *message.Raw, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	for raw := range receiveBuffer {
		w.rawChannel <- raw
	}
}

func (w *PostgresqlWriter) StartMappedWorkers() {
	wg := new(sync.WaitGroup)
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

func (w *PostgresqlWriter) WriteMapped(subscriber *nanomsg.Subscriber[message.Mapped]) {
	receiveBuffer := make(chan *message.Mapped, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	for mapped := range receiveBuffer {
		w.mappedChannel <- mapped
	}
}
