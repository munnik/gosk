package database

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	zapadapter "github.com/jackc/pgx-zap"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// rawDataColumns is both raw_data's COPY column list (see WriteRaw) and
// the column order rawRow's fields must be produced in.
var rawDataColumns = []string{"time", "connector", "value", "type"}

const (
	mappedInsertQuery           = `INSERT INTO "%s" ("time", "connector", "type", "context", "path", "value", "origin") VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT ("time", "origin", "context", "connector", "path") DO UPDATE SET value = EXCLUDED.value`
	selectMappedQuery           = `SELECT "time", "connector", "type", "context", "path", "value", "origin" FROM "mapped_data"`
	selectMostRecentMappedQuery = `SELECT DISTINCT ON ("context", "path") "time", "connector", "type", "context", "path", "value", "origin" FROM "mapped_data" WHERE "time" > $1 ORDER BY "context", "path", "time" DESC`
)

//go:embed migrations/*.sql
var fs embed.FS

// rawRow holds one raw_data row waiting to be COPYed in, in rawDataColumns
// order.
type rawRow struct {
	timestamp time.Time
	connector string
	value     []byte
	type_     string
}

type PostgresqlDatabase struct {
	url             string
	connection      *pgxpool.Pool
	connectionMutex sync.Mutex
	queue           []queuedQuery
	batchSize       int
	rawBatch        []rawRow
	rawBatchMutex   sync.Mutex
	rawFlushMutex   sync.Mutex
	lastFlush       time.Time
	flushMutex      sync.Mutex
	batchMutex      sync.Mutex
	upgradeDone     bool
	databaseTimeout time.Duration
	// backoff paces the flushes after one that failed as a whole, see
	// retryBackoff. One for both queues: a writer process fills one or
	// the other, never both (see cmd/write.go).
	backoff retryBackoff
	// outboxEnabled has WriteSingleValueMapped attach the bucket of every
	// row it queues, see outbox.go; guarded by batchMutex.
	outboxEnabled   bool
	flushesCounter  prometheus.Counter
	lastFlushGauge  prometheus.Gauge
	writesCounter   prometheus.Counter
	timeoutsCounter prometheus.Counter
	dropsCounter    prometheus.Counter
	batchSizeGauge  prometheus.Gauge
}

func NewPostgresqlDatabase(c *config.PostgresqlConfig) *PostgresqlDatabase {
	result := &PostgresqlDatabase{
		url:             c.URLString,
		batchSize:       c.BatchFlushLength,
		lastFlush:       time.Now(),
		upgradeDone:     false,
		databaseTimeout: c.Timeout,
		outboxEnabled:   c.Outbox,
		flushesCounter:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_flushes_total", Help: "total number batches flushed"}),
		lastFlushGauge:  promauto.NewGauge(prometheus.GaugeOpts{Name: "gosk_psql_last_flush_time", Help: "last db flush"}),
		writesCounter:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_writes_total", Help: "total number of deltas added to queue"}),
		timeoutsCounter: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_timeouts_total", Help: "total number timeouts"}),
		dropsCounter:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_drops_total", Help: "total number of deltas dropped because they can never be written"}),
		batchSizeGauge:  promauto.NewGauge(prometheus.GaugeOpts{Name: "gosk_psql_batch_length", Help: "number of deltas in current batch"}),
	}
	go func() {
		ticker := time.NewTicker(c.BatchFlushInterval)
		for {
			<-ticker.C
			if time.Now().After(result.lastFlush.Add(c.BatchFlushInterval)) {
				// A given writer process only ever calls one of WriteRaw or
				// WriteMapped/WriteSingleValueMapped (see cmd/write.go), so
				// exactly one of these two ever has anything queued -
				// calling both here is simpler than tracking which, and the
				// empty one is a no-op (see copyBatch/copyRawBatch).
				result.flushBatch()
				result.flushRawBatch()
			}
		}
	}()
	return result
}

func (db *PostgresqlDatabase) GetConnection() *pgxpool.Pool {
	// check if a connection exist and is pingable, return the connection on success
	if db.connection != nil {
		if err := db.connection.Ping(context.Background()); err == nil {
			return db.connection
		}
	}

	db.connectionMutex.Lock()
	defer db.connectionMutex.Unlock()

	if err := db.UpgradeDatabase(); err != nil {
		logger.GetLogger().Fatal(
			"Could not update the database",
			zap.String("Error", err.Error()),
		)
		return nil
	}

	// check again but now with lock to make sure the connection is not reestablished before acquiring the lock
	if db.connection != nil {
		if err := db.connection.Ping(context.Background()); err == nil {
			return db.connection
		}
	}

	conf, err := pgxpool.ParseConfig(db.url)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not configure the the database connection",
			zap.String("URL", db.url),
			zap.String("Error", err.Error()),
		)
		return nil
	}
	conf.ConnConfig.Tracer = &tracelog.TraceLog{Logger: zapadapter.NewLogger(logger.GetLogger()), LogLevel: tracelog.LogLevelWarn}

	conn, err := pgxpool.NewWithConfig(context.Background(), conf)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the database",
			zap.String("URL", db.url),
			zap.String("Error", err.Error()),
		)
		return nil
	}
	db.connection = conn
	return conn
}

// WriteRaw queues raw for a batched COPY into raw_data (see flushRawBatch),
// rather than an INSERT queued alongside WriteMapped/updateStaticData's
// pgx.Batch: raw_data has no unique constraint or ON CONFLICT clause to
// preserve (unlike mapped_data/static_data's upserts), which makes it a
// clean fit for COPY, Postgres's actual bulk-load path - one execution
// for the whole batch instead of one per row, pipelined or not.
func (db *PostgresqlDatabase) WriteRaw(raw *message.Raw) {
	db.writesCounter.Inc()
	db.rawBatchMutex.Lock()
	db.rawBatch = append(db.rawBatch, rawRow{
		timestamp: raw.Timestamp,
		connector: raw.Connector,
		value:     raw.Value,
		type_:     raw.Type,
	})
	length := len(db.rawBatch)
	db.rawBatchMutex.Unlock()

	db.batchSizeGauge.Inc()
	db.flushRawIfNeeded(length)
}

func (db *PostgresqlDatabase) WriteMapped(mapped *message.Mapped) {
	for _, svm := range mapped.ToSingleValueMapped() {
		db.WriteSingleValueMapped(svm)
	}
}

func (db *PostgresqlDatabase) WriteSingleValueMapped(svm message.SingleValueMapped) {
	db.writesCounter.Inc()
	query, arguments, static, ok := db.mappedInsert(svm)
	if !ok {
		return
	}

	queued := queuedQuery{sql: query, args: arguments}
	db.batchMutex.Lock()
	if static != nil {
		// Ahead of the row, as it always was.
		db.queue = append(db.queue, *static)
	}
	// The row's bucket travels with the row from here on, so that it is
	// marked in the same transaction as the row whichever way the row ends
	// up being written (see pgxBatch and retryIndividually): a row can
	// never exist without its bucket being marked.
	if db.outboxEnabled {
		queued.mark = &outboxMark{origin: svm.Origin, time: svm.Timestamp}
	}
	db.queue = append(db.queue, queued)
	length := len(db.queue)
	db.batchMutex.Unlock()

	db.batchSizeGauge.Inc()
	if static != nil {
		db.batchSizeGauge.Inc()
	}
	db.flushIfNeeded(length)
}

// mappedInsert turns one mapped row into the INSERT that stores it, or
// reports that the row cannot be stored. static, when not nil, is the
// static_data update that goes with it - a vessel's name, mmsi and
// dimensions are kept there as well. Nothing is queued or written here:
// the caller decides where both go, which lets StoreDelivery keep them in
// one transaction with its fence instead of letting a rejected delivery's
// name slip through the writer's queue.
func (db *PostgresqlDatabase) mappedInsert(svm message.SingleValueMapped) (query string, arguments []any, static *queuedQuery, ok bool) {
	path := svm.Path
	if path == "" {
		switch v := svm.Value.(type) {
		case message.VesselInfo:
			if v.MMSI == nil {
				path = "name"
			} else {
				path = "mmsi"
			}
		default:
			logger.GetLogger().Error("unexpected empty path",
				zap.Time("time", svm.Timestamp),
				zap.String("origin", svm.Origin),
				zap.String("context", svm.Context),
				zap.Any("value", svm.Value))
		}
	}

	// before quoting the string
	if sql, value, ok := staticDataQuery(path, svm.Value); ok {
		static = &queuedQuery{sql: sql, args: []any{svm.Context, value}}
	}

	if svm.Value == nil {
		// A bare nil is sent to postgres as SQL NULL, which violates the
		// "value" column's NOT NULL constraint (e.g. a cleared
		// notification, see mapper/notification.go). What's actually
		// wanted is the *JSON* null literal, a valid jsonb value - encode
		// it explicitly instead of letting pgx's default nil handling turn
		// it into a SQL NULL.
		svm.Value = json.RawMessage("null")
	} else if str, ok := svm.Value.(string); ok {
		svm.Value = strconv.Quote(str)
	}

	// The "value" column is jsonb, so pgx encodes this argument with
	// encoding/json. If that cannot be done it fails client-side at flush
	// time, with no SQLSTATE for retryIndividually to recognise as
	// permanent, so the row used to be requeued and retried forever -
	// making it immortal and forcing every later flush down the
	// one-query-at-a-time path, which is what wedged a vessel's writer for
	// ten hours. Marshal it here instead, where the path and source are
	// still around to name whichever sensor or expression produced it.
	//
	// The realistic cause is a non-finite float, which JSON cannot
	// represent. runExpr already rejects an expression that evaluates to a
	// bare NaN or Inf (see mapper/expressions.go), but only at the top
	// level: one nested in a struct - a Position's latitude, say - reaches
	// here untouched.
	if _, err := json.Marshal(svm.Value); err != nil {
		db.dropsCounter.Inc()
		logger.GetLogger().Error(
			"Dropping a value that cannot be encoded as jsonb",
			zap.String("value", fmt.Sprint(svm.Value)),
			zap.Time("time", svm.Timestamp),
			zap.String("path", path),
			zap.String("context", svm.Context),
			zap.String("origin", svm.Origin),
			zap.String("source.label", svm.Source.Label),
			zap.String("source.type", svm.Source.Type),
			zap.Error(err),
		)
		return "", nil, nil, false
	}

	table := "mapped_data_matching_context"
	if svm.Context != svm.Origin {
		table = "mapped_data_other_context"
	}
	query = fmt.Sprintf(mappedInsertQuery, table)
	arguments = []any{svm.Timestamp, svm.Source.Label, svm.Source.Type, svm.Context, path, svm.Value, svm.Origin}
	return query, arguments, static, true
}

// staticDataQuery is the static_data upsert for a mapped value whose path
// is kept there, taking the context and the value as its two arguments,
// or ok false for any other path.
func staticDataQuery(path string, value any) (query string, argument any, ok bool) {
	var v any
	var column string
	switch path {
	case "mmsi":
		if vi, ok := value.(message.VesselInfo); ok {
			v = vi.MMSI
			column = "mmsi"
		}
	case "name":
		if vi, ok := value.(message.VesselInfo); ok {
			v = vi.Name
			column = "name"
		}
	case "communication.callsignVhf":
		v = value.(string)
		column = "callsignvhf"
	case "registrations.other.eni.registration":
		v = value.(string)
		column = "eninumber"
	case "design.length":
		if l, ok := value.(message.Length); ok {
			v = l.Overall
			column = "length"
		}
	case "design.beam":
		v = value
		column = "beam"
	case "design.aisShipType":
		if vt, ok := value.(message.VesselType); ok {
			v = vt.Description
			column = "vesseltype"
		}
	}
	if column == "" {
		return "", nil, false // the path is not static data
	}
	if v == nil {
		logger.GetLogger().Warn("Could not update static data, check path and value",
			zap.String("path", path),
			zap.Any("value", value),
		)
		return "", nil, false
	}

	query = fmt.Sprintf(`INSERT INTO "static_data" ("context", "%s") VALUES ($1, $2) ON CONFLICT("context") DO UPDATE SET "%s" = EXCLUDED."%s";`, column, column, column)
	return query, v, true
}

func (db *PostgresqlDatabase) ReadMostRecentMapped(fromTime time.Time) ([]*message.Mapped, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout*10)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectMostRecentMappedQuery, fromTime)

	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeoutsCounter.Inc()
		return nil, ctx.Err()
	}
	defer rows.Close()

	result := make([]*message.Mapped, 0)
	for rows.Next() {
		m := message.NewSingleValueMapped()
		err := rows.Scan(
			&m.Timestamp,
			&m.Source.Label,
			&m.Source.Type,
			&m.Context,
			&m.Path,
			&m.Value,
			&m.Origin,
		)
		if err != nil {
			return nil, err
		}
		if m.Value, err = message.Decode(m.Value); err != nil {
			logger.GetLogger().Warn(
				"Could not decode value",
				zap.String("Error", err.Error()),
				zap.Any("Value", m.Value),
			)
		}
		if m.Path == "mmsi" || m.Path == "name" {
			m.Path = ""
		}
		result = append(result, m.ToMapped())
	}
	// check for errors after last call to .Next()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (db *PostgresqlDatabase) ReadMapped(appendToQuery string, arguments ...interface{}) ([]*message.Mapped, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	rows, err := db.readSingleValueMapped(ctx, db.GetConnection(), appendToQuery, arguments...)
	if err != nil {
		return nil, err
	}
	result := make([]*message.Mapped, 0, len(rows))
	for i := range rows {
		result = append(result, rows[i].ToMapped())
	}
	return result, nil
}

// queryer is what readSingleValueMapped needs from a connection, so it can
// read inside a transaction (the outbox sender, see outbox.go) as well as
// straight from the pool.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// readSingleValueMapped runs selectMappedQuery with appendToQuery after
// it and decodes the rows, one SingleValueMapped per row.
func (db *PostgresqlDatabase) readSingleValueMapped(ctx context.Context, q queryer, appendToQuery string, arguments ...interface{}) ([]message.SingleValueMapped, error) {
	rows, err := q.Query(ctx, fmt.Sprintf("%s %s", selectMappedQuery, appendToQuery), arguments...)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeoutsCounter.Inc()
		return nil, ctx.Err()
	}
	defer rows.Close()

	result := make([]message.SingleValueMapped, 0)
	for rows.Next() {
		m := message.NewSingleValueMapped()
		err := rows.Scan(
			&m.Timestamp,
			&m.Source.Label,
			&m.Source.Type,
			&m.Context,
			&m.Path,
			&m.Value,
			&m.Origin,
		)
		if err != nil {
			return nil, err
		}
		if m.Value, err = message.Decode(m.Value); err != nil {
			logger.GetLogger().Warn(
				"Could not decode value",
				zap.String("Error", err.Error()),
				zap.Any("Value", m.Value),
			)
		}
		if m.Path == "mmsi" || m.Path == "name" {
			m.Path = ""
		}
		result = append(result, *m)
	}
	// check for errors after last call to .Next()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (db *PostgresqlDatabase) UpgradeDatabase() error {
	if db.upgradeDone {
		return nil
	}

	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, db.url)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	db.upgradeDone = true
	return nil
}

func (db *PostgresqlDatabase) DowngradeDatabase() error {
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, db.url)
	if err != nil {
		return err
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

// maxBatchBacklogFactor bounds how far the batch can grow past batchSize
// before flushIfNeeded blocks its caller on a synchronous flush instead of
// just triggering one in the background and returning immediately: past
// that point a database that's fallen behind (or stalled) needs to slow
// its writer down, not let it keep queuing indefinitely. Write*/
// updateStaticData's caller (writer.PostgresqlWriter.Write) processes
// messages one at a time, so blocking here directly paces how fast it
// drains its subscriber's buffer - which then falls back to dropping data
// once full, a safe failure mode, instead of this process's memory
// growing without limit until the kernel OOM-kills it.
const maxBatchBacklogFactor = 4

const (
	// minRetryBackoff is how long the first flush after a failed one
	// waits; every further failure doubles it, up to maxRetryBackoff, and a
	// flush that succeeds resets it. Short, because the queue it holds back
	// is in memory and bounded only by the backpressure of flushIfNeeded -
	// the longer the wait, the more of the subscriber's buffer upstream is
	// dropped on a database that is in fact back.
	minRetryBackoff = time.Second
	maxRetryBackoff = 30 * time.Second
)

// retryBackoff holds back the next flush after one that failed for a reason
// that lies with the database or the connection rather than with any one
// query - a timeout, a database that cannot be reached. Such a batch is
// requeued whole: retrying it query by query, as a batch that has a bad
// query in it is (see retryIndividually), would have each of its rows take
// a fresh timeout in turn, with the flush lock held and the writers behind
// it blocked, for as long as the database stays slow. Retrying it right
// away would be the next flush failing the same way: every write past the
// batch length starts a flush, so a vessel's writer would be hammering a
// database that is not answering, once per delta.
type retryBackoff struct {
	mutex sync.Mutex
	delay time.Duration
	until time.Time
}

// wait blocks until the backoff set by the last extend has passed.
func (b *retryBackoff) wait() {
	b.mutex.Lock()
	until := b.until
	b.mutex.Unlock()
	if remaining := time.Until(until); remaining > 0 {
		time.Sleep(remaining)
	}
}

// extend holds the next flush back, for twice as long as the last time up
// to maxRetryBackoff, and returns for how long.
func (b *retryBackoff) extend() time.Duration {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.delay == 0 {
		b.delay = minRetryBackoff
	} else {
		b.delay = min(2*b.delay, maxRetryBackoff)
	}
	b.until = time.Now().Add(b.delay)
	return b.delay
}

// reset forgets the backoff after a flush that succeeded.
func (b *retryBackoff) reset() {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.delay = 0
	b.until = time.Time{}
}

func (db *PostgresqlDatabase) flushIfNeeded(length int) {
	switch {
	case length > db.batchSize*maxBatchBacklogFactor:
		db.flushBatch()
	case length > db.batchSize:
		go db.flushBatch()
	}
}

// flushRawIfNeeded is flushIfNeeded's counterpart for the raw_data COPY
// batch (see WriteRaw/flushRawBatch) - same threshold, same backpressure
// reasoning.
func (db *PostgresqlDatabase) flushRawIfNeeded(length int) {
	switch {
	case length > db.batchSize*maxBatchBacklogFactor:
		db.flushRawBatch()
	case length > db.batchSize:
		go db.flushRawBatch()
	}
}

func (db *PostgresqlDatabase) flushBatch() {
	// Serialize flushes (the ticker in NewPostgresqlDatabase and the
	// batch.Len() > batchSize check in Write*/updateStaticData can both
	// trigger one concurrently). Overlapping SendBatch calls sharing the
	// same pool were the likely cause of "prepared statement does not
	// exist" errors that then looped forever below (see invalidateConnection).
	db.flushMutex.Lock()
	defer db.flushMutex.Unlock()
	// Holding the lock while waiting is the point: the flush that a writer
	// past the backlog limit runs in its own goroutine (flushIfNeeded)
	// waits here with it, which is what keeps the queue bounded while the
	// database is away.
	db.backoff.wait()

	queries := db.copyBatch()
	// prevent flushing when queue is empty
	if queries == nil {
		return
	}
	uuid := uuid.New()
	start := time.Now()
	logger.GetLogger().Info(
		"Going to flush",
		zap.String("uuid", uuid.String()),
	)
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	result := db.GetConnection().SendBatch(ctx, db.pgxBatch(queries))

	if err := result.Close(); err != nil {
		// The failure could be caused by connection/session state (e.g. a
		// prepared statement cached against a connection that's since been
		// replaced or reset server-side). Retrying against that same
		// connection would just fail identically forever, silently, since
		// nothing downstream of Write*'s queue-then-return observes these
		// retries. Drop it so the retries below (and the next flush) get a
		// fresh one.
		db.invalidateConnection()
		if ctx.Err() != nil {
			// The batch timed out as a whole - the database is slow, or
			// one of its outbox upserts waited on a bucket the sender had
			// locked. Nothing of it committed (one transaction), and
			// nothing of it is known to be unwritable, so it goes back
			// into the queue whole, ahead of what was written since, for
			// a later flush to try again - rather than discarded, as it
			// used to be, or retried query by query, which would have
			// every row wait out its own timeout. Every query is
			// idempotent (ON CONFLICT, or an upsert), so one that did
			// commit after all is harmless to repeat.
			db.timeoutsCounter.Inc()
			logger.GetLogger().Error(
				"Timeout during database insertion, requeuing the batch",
				zap.Int("Queries", len(queries)),
				zap.Duration("RetryAfter", db.backoff.extend()),
				zap.Error(ctx.Err()),
				zap.Error(err),
			)
			db.requeue(queries)
			return
		}
		logger.GetLogger().Error(
			"Unable to flush batch as a whole, retrying its queries individually",
			zap.String("Error", err.Error()),
		)
		db.retryIndividually(queries)
		return
	}
	db.backoff.reset()
	logger.GetLogger().Info(
		"Flush completed",
		zap.String("uuid", uuid.String()),
		zap.Duration("duration", time.Since(start)),
	)
	db.lastFlush = time.Now()
	db.lastFlushGauge.SetToCurrentTime()
}

// flushRawBatch COPYs the queued raw_data rows in one execution, rather
// than the one-query-per-row approach flushBatch takes for mapped_data/
// static_data's upserts (see WriteRaw's doc comment for why COPY doesn't
// fit those). Unlike retryIndividually, a failed COPY has no per-row
// retry: it's an all-or-nothing statement, and raw_data has no constraint
// a row could permanently violate the way an upsert or a NOT NULL column
// elsewhere might, so requeuing the whole batch on failure (the same
// connection-invalidate-and-retry reasoning as flushBatch's) is enough.
// That goes for a timeout as much as for any other failure: nothing of a
// COPY that was cut short committed. (raw_data has no key, so a COPY that
// did commit just as it was given up on is stored twice. Rare, and raw
// data is kept for looking into, not for counting.)
func (db *PostgresqlDatabase) flushRawBatch() {
	db.rawFlushMutex.Lock()
	defer db.rawFlushMutex.Unlock()
	db.backoff.wait()

	rows := db.copyRawBatch()
	if rows == nil {
		return
	}
	uuid := uuid.New()
	start := time.Now()
	logger.GetLogger().Info(
		"Going to flush",
		zap.String("uuid", uuid.String()),
	)
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	_, err := db.GetConnection().CopyFrom(
		ctx,
		pgx.Identifier{"raw_data"},
		rawDataColumns,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{r.timestamp, r.connector, r.value, r.type_}, nil
		}),
	)
	if err != nil {
		db.invalidateConnection()
		retryAfter := db.backoff.extend()
		if ctx.Err() != nil {
			db.timeoutsCounter.Inc()
			logger.GetLogger().Error(
				"Timeout during database insertion, requeuing the raw_data batch",
				zap.Int("Rows", len(rows)),
				zap.Duration("RetryAfter", retryAfter),
				zap.Error(ctx.Err()),
				zap.Error(err),
			)
		} else {
			logger.GetLogger().Error(
				"Unable to COPY the raw_data batch, requeuing it",
				zap.Int("Rows", len(rows)),
				zap.Duration("RetryAfter", retryAfter),
				zap.String("Error", err.Error()),
			)
		}
		db.rawBatchMutex.Lock()
		db.rawBatch = append(rows, db.rawBatch...)
		db.rawBatchMutex.Unlock()
		db.batchSizeGauge.Add(float64(len(rows)))
		return
	}
	db.backoff.reset()
	logger.GetLogger().Info(
		"Flush completed",
		zap.String("uuid", uuid.String()),
		zap.Duration("duration", time.Since(start)),
	)
	db.lastFlush = time.Now()
	db.lastFlushGauge.SetToCurrentTime()
}

// invalidateConnection closes and discards the current connection pool, so
// the next GetConnection call establishes a fresh one instead of reusing a
// connection that may be left in a bad session state.
func (db *PostgresqlDatabase) invalidateConnection() {
	db.connectionMutex.Lock()
	defer db.connectionMutex.Unlock()
	if db.connection != nil {
		db.connection.Close()
		db.connection = nil
	}
}

// queuedQuery is one statement waiting for the next flush. For a mapped
// row it also carries the outbox bucket the row belongs to, so that
// whichever path writes the row - the batch, or the query-by-query retry
// of a batch that failed - writes the mark with it, in one transaction. The
// two must never be separated: a row stored with nothing marking its bucket
// is never sent, and a mark committed ahead of its row can be read, found
// empty and acknowledged before the row lands (see outbox.go).
type queuedQuery struct {
	sql  string
	args []any
	mark *outboxMark
}

// retryIndividually re-sends each query from a batch that failed as a
// whole, one at a time, and sorts the failures. A query that fails with a
// Postgres data or integrity error (SQLSTATE class 22 or 23 - the kind
// that will never succeed no matter how many times it's retried) is logged
// and dropped, as is one whose arguments fail to encode, which never
// reaches postgres at all. Any other failure (connectivity, a still-stale
// prepared statement, a timeout, ...) stops the retry: that query and
// every one after it, untried, go back into the queue in their order, and
// the next flush is held back (see retryBackoff). Carrying on past such a
// failure would let a later write to the same key commit before the
// failed one, which is then requeued and overwrites it with the older
// value on the next flush. (After a timeout the ones after it would only
// have timed out in turn, too.)
//
// A mapped row is written together with its outbox mark, in a transaction
// of its own (execWithMark), so that this path gives the same guarantee as
// the batch: the row commits with its bucket marked or not at all.
func (db *PostgresqlDatabase) retryIndividually(queries []queuedQuery) {
	for i, query := range queries {
		ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
		var err error
		if query.mark != nil {
			err = db.execWithMark(ctx, query)
		} else {
			_, err = db.GetConnection().Exec(ctx, query.sql, query.args...)
		}
		timedOut := ctx.Err() != nil
		cancel()
		if err == nil {
			continue
		}
		if !timedOut && db.dropIfUnwritable(query, err) {
			continue
		}

		if timedOut {
			db.timeoutsCounter.Inc()
		}
		logger.GetLogger().Error(
			"A query failed even individually, reinserting it and the rest of the batch into the queue",
			zap.Int("Untried", len(queries)-i-1),
			zap.Duration("RetryAfter", db.backoff.extend()),
			zap.Error(err),
		)
		db.requeue(queries[i:])
		return
	}
	db.backoff.reset()
}

// dropIfUnwritable reports whether err means query can never be written,
// and logs and counts the drop when so.
func (db *PostgresqlDatabase) dropIfUnwritable(query queuedQuery, err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (strings.HasPrefix(pgErr.Code, "22") || strings.HasPrefix(pgErr.Code, "23")) {
		db.dropsCounter.Inc()
		logger.GetLogger().Error(
			"Dropping a query that permanently fails with a data/integrity error, it would otherwise block every other queued query forever",
			zap.String("sql", query.sql),
			zap.String("sqlstate", pgErr.Code),
			zap.Error(err),
		)
		return true
	}

	// An argument that fails to encode never reached the server, so
	// there's no PgError to classify above - and because encoding is a
	// pure function of the arguments, it will fail identically on every
	// future flush. Requeuing such a row makes it immortal, and every
	// flush that carries it then fails as a batch and comes back here to
	// retry its queries one at a time; that is what wedged a vessel's
	// writer for ten hours on a single NaN. WriteSingleValueMapped
	// marshals a mapped value before queueing it, so this is the
	// backstop for every other query - static_data's, and anything a
	// future caller queues without that check.
	var unsupportedValue *json.UnsupportedValueError
	var unsupportedType *json.UnsupportedTypeError
	var marshalerError *json.MarshalerError
	if errors.As(err, &unsupportedValue) || errors.As(err, &unsupportedType) || errors.As(err, &marshalerError) {
		db.dropsCounter.Inc()
		logger.GetLogger().Error(
			"Dropping a query whose arguments cannot be encoded, it would otherwise block every other queued query forever",
			zap.String("sql", query.sql),
			// Not zap.Any: encoding these is what just failed, so
			// let fmt render them instead of json.
			zap.String("arguments", fmt.Sprint(query.args)),
			zap.Error(err),
		)
		return true
	}
	return false
}

// requeue puts queries back at the head of the queue, ahead of whatever
// was written while they were being flushed. At the tail they would be
// flushed after a newer write to the same key, and a mapped_data upsert
// takes the value it is handed, not the newest, so the older one would end
// up stored.
func (db *PostgresqlDatabase) requeue(queries []queuedQuery) {
	if len(queries) == 0 {
		return
	}
	db.batchMutex.Lock()
	merged := make([]queuedQuery, 0, len(queries)+len(db.queue))
	merged = append(merged, queries...)
	merged = append(merged, db.queue...)
	db.queue = merged
	db.batchMutex.Unlock()
	db.batchSizeGauge.Add(float64(len(queries)))
}

// copyBatch takes everything queued, leaving the queue empty; nil when
// there was nothing.
func (db *PostgresqlDatabase) copyBatch() []queuedQuery {
	db.batchMutex.Lock()
	defer db.batchMutex.Unlock()

	if len(db.queue) < 1 {
		return nil
	}

	db.flushesCounter.Inc()
	queries := db.queue
	db.queue = nil
	db.batchSizeGauge.Set(0)
	return queries
}

// pgxBatch is what a flush sends: the queries, then one outbox upsert per
// bucket their rows mark. One pgx batch runs as one implicit transaction,
// so a row never commits without its mark, nor a mark without its rows.
func (db *PostgresqlDatabase) pgxBatch(queries []queuedQuery) *pgx.Batch {
	batch := &pgx.Batch{}
	queue := func(q queuedQuery) {
		batch.Queue(q.sql, q.args...).Exec(func(ct pgconn.CommandTag) error {
			if ct.RowsAffected() == 0 {
				logger.GetLogger().Warn("0 rows affected",
					zap.String("query", q.sql),
					zap.Any("arguments", q.args),
				)
			}
			return nil
		})
	}
	for _, q := range queries {
		queue(q)
	}
	for _, q := range outboxUpserts(queries) {
		queue(q)
	}
	return batch
}

// copyRawBatch is copyBatch's counterpart for the raw_data COPY batch: nil
// means nothing to flush, matching copyBatch's nil-Batch convention.
func (db *PostgresqlDatabase) copyRawBatch() []rawRow {
	db.rawBatchMutex.Lock()
	defer db.rawBatchMutex.Unlock()

	if len(db.rawBatch) == 0 {
		return nil
	}

	db.flushesCounter.Inc()
	rows := db.rawBatch
	db.rawBatch = nil
	db.batchSizeGauge.Set(0)

	return rows
}
