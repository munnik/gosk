package writer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type PostgresqlWriter[T nanomsg.Message] struct {
	db             *database.PostgresqlDatabase
	writtenCounter prometheus.Counter
}

func NewPostgresqlWriter[T nanomsg.Message](c *config.PostgresqlConfig) *PostgresqlWriter[T] {
	return &PostgresqlWriter[T]{
		db:             database.NewPostgresqlDatabase(c),
		writtenCounter: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_messages_written_total", Help: "total number of nano messages sent to db"}),
	}
}

func (w *PostgresqlWriter[T]) Write(subscriber *nanomsg.Subscriber[T]) {
	receiveBuffer := make(chan *T, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	if receiveBufferRaw, ok := any(receiveBuffer).(chan *message.Raw); ok {
		for raw := range receiveBufferRaw {
			go func(raw *message.Raw) {
				w.db.WriteRaw(raw)
				w.writtenCounter.Inc()
				subscriber.ReturnToPool(any(raw).(*T))
			}(raw)
		}
	}
	if receiveBufferMapped, ok := any(receiveBuffer).(chan *message.Mapped); ok {
		for mapped := range receiveBufferMapped {
			go func(mapped *message.Mapped) {
				w.db.WriteMapped(mapped)
				w.writtenCounter.Inc()
				subscriber.ReturnToPool(any(mapped).(*T))
			}(mapped)
		}
	}
}
