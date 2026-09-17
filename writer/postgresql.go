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

// Write processes messages from subscriber sequentially, one at a time -
// deliberately not a goroutine per message. WriteRaw/WriteMapped only ever
// queue into an in-memory batch and return; nothing here waits on the
// database except the batch's own periodic/threshold flush. Spawning a
// goroutine per incoming message on top of that added no throughput (the
// batch mutex serializes the actual queuing anyway) while removing the one
// thing that kept this loop's pace tied to how fast the database can
// drain: with an unbounded number of in-flight goroutines, a database that
// falls behind (or stalls) no longer slows this loop down at all, so the
// batch - and this process's memory - grows without limit until the kernel
// OOM-kills it. Processing sequentially, combined with
// PostgresqlDatabase's own backpressure once its batch backlog grows too
// far past its configured flush size (see flushIfNeeded), means a slow
// database instead makes this loop fall behind, which engages
// subscriber's own bounded buffer (it drops data once full rather than
// blocking its sender) - a much safer failure mode than an OOM crash loop.
func (w *PostgresqlWriter[T]) Write(subscriber *nanomsg.Subscriber[T]) {
	receiveBuffer := make(chan *T, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	if receiveBufferRaw, ok := any(receiveBuffer).(chan *message.Raw); ok {
		for raw := range receiveBufferRaw {
			w.db.WriteRaw(raw)
			w.writtenCounter.Inc()
		}
	}
	if receiveBufferMapped, ok := any(receiveBuffer).(chan *message.Mapped); ok {
		for mapped := range receiveBufferMapped {
			w.db.WriteMapped(mapped)
			w.writtenCounter.Inc()
		}
	}
}
