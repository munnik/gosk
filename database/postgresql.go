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
	zapadapter "github.com/jackc/pgx-zap"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// rawDataColumns is both raw_data's COPY column list (see WriteRaw) and
// the column order rawRow's fields must be produced in.
var rawDataColumns = []string{"time", "connector", "value", "uuid", "type"}

const (
	mappedInsertQuery              = `INSERT INTO "%s" ("time", "connector", "type", "context", "path", "value", "uuid", "origin", "transfer_uuid") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT ("time", "origin", "context", "connector", "path") DO UPDATE SET value = EXCLUDED.value`
	selectMappedQuery              = `SELECT "time", "connector", "type", "context", "path", "value", "uuid", "origin", "transfer_uuid" FROM "mapped_data"`
	selectMostRecentMappedQuery    = `SELECT DISTINCT ON ("context", "path") "time", "connector", "type", "context", "path", "value", "uuid", "origin", "transfer_uuid" FROM "mapped_data" WHERE "time" > $1 ORDER BY "context", "path", "time" DESC`
	selectLocalCountQuery          = `SELECT "count" FROM "transfer_local_data" WHERE "origin" = $1 AND "start" = $2`
	selectExistingRemoteCounts     = `SELECT "origin", "start" FROM "transfer_remote_data" WHERE "start" >= $1`
	selectIncompletePeriodsQuery   = `SELECT "origin", "start", "local_count", "remote_count" FROM "transfer_data" WHERE "local_count" < "remote_count" * $1::double precision ORDER BY "start" DESC`
	insertOrUpdateRemoteData       = `INSERT INTO "transfer_remote_data" ("start", "origin", "count") VALUES ($1, $2, $3) ON CONFLICT ("start", "origin") DO UPDATE SET "count" = EXCLUDED.count`
	logTransferInsertQuery         = `INSERT INTO "transfer_log" ("time", "origin", "message") VALUES ($1, $2, $3)`
	selectMappedCountPerUuid       = `SELECT "uuid", COUNT("uuid") FROM "mapped_data" WHERE "origin" = $1 AND "time" BETWEEN $2 AND $2 + '5m'::interval GROUP BY 1`
	selectFirstMappedDataPerOrigin = `SELECT "origin", MIN("start") FROM "transfer_local_data" GROUP BY 1`
)

//go:embed migrations/*.sql
var fs embed.FS

type IncompletePeriod struct {
	Origin      string
	Period      time.Time
	LocalCount  int
	RemoteCount int
}

// rawRow holds one raw_data row waiting to be COPYed in, in rawDataColumns
// order.
type rawRow struct {
	timestamp time.Time
	connector string
	value     []byte
	uuid      uuid.UUID
	type_     string
}

type PostgresqlDatabase struct {
	url             string
	connection      *pgxpool.Pool
	connectionMutex sync.Mutex
	batch           *pgx.Batch
	batchSize       int
	rawBatch        []rawRow
	rawBatchMutex   sync.Mutex
	rawFlushMutex   sync.Mutex
	lastFlush       time.Time
	flushMutex      sync.Mutex
	batchMutex      sync.Mutex
	upgradeDone     bool
	databaseTimeout time.Duration
	flushesCounter  prometheus.Counter
	lastFlushGauge  prometheus.Gauge
	writesCounter   prometheus.Counter
	timeoutsCounter prometheus.Counter
	batchSizeGauge  prometheus.Gauge
}

func NewPostgresqlDatabase(c *config.PostgresqlConfig) *PostgresqlDatabase {
	result := &PostgresqlDatabase{
		url:             c.URLString,
		batchSize:       c.BatchFlushLength,
		batch:           &pgx.Batch{},
		lastFlush:       time.Now(),
		upgradeDone:     false,
		databaseTimeout: c.Timeout,
		flushesCounter:  promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_flushes_total", Help: "total number batches flushed"}),
		lastFlushGauge:  promauto.NewGauge(prometheus.GaugeOpts{Name: "gosk_psql_last_flush_time", Help: "last db flush"}),
		writesCounter:   promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_writes_total", Help: "total number of deltas added to queue"}),
		timeoutsCounter: promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_timeouts_total", Help: "total number timeouts"}),
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
		uuid:      raw.Uuid,
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

	// run this before quoting the string
	db.updateStaticData(svm.Context, path, svm.Value)

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
	table := "mapped_data_matching_context"
	if svm.Context != svm.Origin {
		table = "mapped_data_other_context"
	}
	query := fmt.Sprintf(mappedInsertQuery, table)
	db.batchMutex.Lock()
	db.batch.Queue(query, svm.Timestamp, svm.Source.Label, svm.Source.Type, svm.Context, path, svm.Value, svm.Source.Uuid, svm.Origin, svm.Source.TransferUuid).Exec(func(ct pgconn.CommandTag) error {
		if ct.RowsAffected() == 0 {
			logger.GetLogger().Warn("0 rows affected",
				zap.String("query", query),
				zap.Time("timestamp", svm.Timestamp),
				zap.String("source.label", svm.Source.Label),
				zap.String("source.type", svm.Source.Type),
				zap.String("context", svm.Context),
				zap.String("path", path),
				zap.Any("value", svm.Value),
				zap.String("source.uuid", svm.Source.Uuid.String()),
				zap.String("origin", svm.Origin),
				zap.String("source.transferuuid", svm.Source.TransferUuid.String()),
			)
		}

		return nil
	})
	length := db.batch.Len()
	db.batchMutex.Unlock()

	db.batchSizeGauge.Inc()
	db.flushIfNeeded(length)
}

func (db *PostgresqlDatabase) updateStaticData(context, path string, value any) {
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
		return // column is not set, so the path is not static data
	}
	if v == nil {
		logger.GetLogger().Warn("Could not update static data, check path and value",
			zap.String("path", path),
			zap.Any("value", value),
		)
		return
	}

	query := fmt.Sprintf(`INSERT INTO "static_data" ("context", "%s") VALUES ($1, $2) ON CONFLICT("context") DO UPDATE SET "%s" = EXCLUDED."%s";`, column, column, column)
	db.batchMutex.Lock()
	db.batch.Queue(query, context, v).Exec(func(ct pgconn.CommandTag) error {
		if ct.RowsAffected() == 0 {
			logger.GetLogger().Warn("0 rows affected",
				zap.String("query", query),
				zap.String("context", context),
				zap.Any("v", v),
			)
		}

		return nil
	})
	length := db.batch.Len()
	db.batchMutex.Unlock()

	db.batchSizeGauge.Inc()
	db.flushIfNeeded(length)
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
			&m.Source.Uuid,
			&m.Origin,
			&m.Source.TransferUuid,
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
	rows, err := db.GetConnection().Query(ctx, fmt.Sprintf("%s %s", selectMappedQuery, appendToQuery), arguments...)
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
			&m.Source.Uuid,
			&m.Origin,
			&m.Source.TransferUuid,
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

// Returns the start timestamp of each period that has local count but no remote count
func (db *PostgresqlDatabase) SelectFirstMappedDataPerOrigin() (map[string]time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectFirstMappedDataPerOrigin)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeoutsCounter.Inc()
		return nil, ctx.Err()
	}
	defer rows.Close()

	result := make(map[string]time.Time)
	var origin string
	var minStart time.Time
	for rows.Next() {
		err := rows.Scan(&origin, &minStart)
		if err != nil {
			return nil, err
		}
		result[origin] = minStart
	}
	// check for errors after last call to .Next()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (db *PostgresqlDatabase) SelectExistingRemoteCounts(from time.Time) (map[string]map[time.Time]struct{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectExistingRemoteCounts, from)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeoutsCounter.Inc()
		return nil, ctx.Err()
	}
	defer rows.Close()

	result := make(map[string]map[time.Time]struct{})
	var origin string
	var start time.Time
	for rows.Next() {
		err := rows.Scan(&origin, &start)
		if err != nil {
			return nil, err
		}
		if _, ok := result[origin]; !ok {
			result[origin] = make(map[time.Time]struct{})
		}
		result[origin][start] = struct{}{}
	}
	// check for errors after last call to .Next()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// Returns the start timestamp of each period that has the same or more local rows than remote
func (db *PostgresqlDatabase) SelectIncompletePeriods(completenessFactor float64) ([]IncompletePeriod, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectIncompletePeriodsQuery, completenessFactor)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeoutsCounter.Inc()
		return nil, ctx.Err()
	}
	defer rows.Close()

	result := make([]IncompletePeriod, 0)
	var origin string
	var period time.Time
	var localCount int
	var remoteCount int
	for rows.Next() {
		err := rows.Scan(&origin, &period, &localCount, &remoteCount)
		if err != nil {
			return nil, err
		}
		result = append(result, IncompletePeriod{
			Origin:      origin,
			Period:      period,
			LocalCount:  localCount,
			RemoteCount: remoteCount,
		})
	}
	// check for errors after last call to .Next()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (db *PostgresqlDatabase) SelectCountMapped(origin string, start time.Time) (int, error) {
	var result int
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	err := db.GetConnection().QueryRow(ctx, selectLocalCountQuery, origin, start).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		logger.GetLogger().Warn(
			"No rows found so returning 0 count",
			zap.Error(err),
		)
		return 0, nil
	}
	if err != nil {
		return 0, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeoutsCounter.Inc()
		return 0, ctx.Err()
	}

	return result, nil
}

// Return the number of rows per (raw) uuid in the mapped_data table
func (db *PostgresqlDatabase) SelectCountPerUuid(origin string, start time.Time) (map[uuid.UUID]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectMappedCountPerUuid, origin, start)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeoutsCounter.Inc()
		return nil, ctx.Err()
	}
	defer rows.Close()

	result := make(map[uuid.UUID]int)
	var uuid uuid.UUID
	var count int
	for rows.Next() {
		err := rows.Scan(&uuid, &count)
		if err != nil {
			return nil, err
		}
		result[uuid] = count
	}
	// check for errors after last call to .Next()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// Creates a period in the database, the default value for the local data points is -1
func (db *PostgresqlDatabase) CreateRemoteCount(start time.Time, origin string, count int) error {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	_, err := db.GetConnection().Exec(ctx, insertOrUpdateRemoteData, start, origin, count)
	if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database insertion")
		db.timeoutsCounter.Inc()
		return ctx.Err()
	}
	return err
}

// LogTransferRequest queues one transfer_log row onto the same batch
// Write*/updateStaticData use, rather than sending it immediately.
//
// It used to be one round trip per call, at the full databaseTimeout (the
// transfer requester configures five minutes). That is one round trip per
// request *and* per response, on a process whose whole job is to send
// thousands of them - a backlog of a few hundred periods across a handful
// of origins spent most of its time waiting on this rather than on the
// requests themselves. Nothing reads these rows synchronously and every
// caller already discards the result, so there is nothing to be gained by
// making the caller wait for it.
//
// The timestamp is passed explicitly because the row is now written some
// time after the event: NOW() would record the flush.
func (db *PostgresqlDatabase) LogTransferRequest(origin string, message interface{}) {
	db.batchMutex.Lock()
	db.batch.Queue(logTransferInsertQuery, time.Now(), origin, message)
	length := db.batch.Len()
	db.batchMutex.Unlock()

	db.batchSizeGauge.Inc()
	db.flushIfNeeded(length)
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

	batchToFlush := db.copyBatch()
	// prevent flushing when queue is empty
	if batchToFlush == nil {
		return
	}
	uuid := uuid.Must(uuid.NewV7Precise())
	start := time.Now()
	logger.GetLogger().Info(
		"Going to flush",
		zap.String("uuid", uuid.String()),
	)
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	result := db.GetConnection().SendBatch(ctx, batchToFlush)

	if err := result.Close(); err != nil {
		if ctx.Err() != nil {
			logger.GetLogger().Error("Timeout during database insertion", zap.Error(ctx.Err()), zap.Error(err))
			db.timeoutsCounter.Inc()
			return
		}
		logger.GetLogger().Error(
			"Unable to flush batch as a whole, retrying its queries individually",
			zap.String("Error", err.Error()),
		)
		// The failure could be caused by connection/session state (e.g. a
		// prepared statement cached against a connection that's since been
		// replaced or reset server-side). Retrying against that same
		// connection would just fail identically forever, silently, since
		// nothing downstream of Write*'s queue-then-return observes these
		// retries. Drop it so the retries below (and the next flush) get a
		// fresh one.
		db.invalidateConnection()
		db.retryIndividually(batchToFlush)
		return
	}
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
func (db *PostgresqlDatabase) flushRawBatch() {
	db.rawFlushMutex.Lock()
	defer db.rawFlushMutex.Unlock()

	rows := db.copyRawBatch()
	if rows == nil {
		return
	}
	uuid := uuid.Must(uuid.NewV7Precise())
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
			return []any{r.timestamp, r.connector, r.value, r.uuid, r.type_}, nil
		}),
	)
	if err != nil {
		if ctx.Err() != nil {
			logger.GetLogger().Error("Timeout during database insertion", zap.Error(ctx.Err()), zap.Error(err))
			db.timeoutsCounter.Inc()
			return
		}
		logger.GetLogger().Error(
			"Unable to COPY the raw_data batch, requeuing it",
			zap.String("Error", err.Error()),
		)
		db.invalidateConnection()
		db.rawBatchMutex.Lock()
		db.rawBatch = append(rows, db.rawBatch...)
		db.rawBatchMutex.Unlock()
		return
	}
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

// retryIndividually re-sends each query from a batch that failed as a
// whole, one at a time. A single permanently failing query (e.g. a NOT
// NULL or other constraint violation) would otherwise keep the *entire*
// batch requeued and retried forever, blocking every other query queued
// alongside it - including brand new, unrelated ones, since they all share
// the same underlying queue. Queries whose failure is classified as a
// Postgres data or integrity error (SQLSTATE class 22 or 23 - the kind
// that will never succeed no matter how many times it's retried) are
// logged and dropped. Anything else (connectivity issues, a still-stale
// prepared statement, timeouts, ...) is requeued for the next flush, same
// as the whole-batch retry used to do.
func (db *PostgresqlDatabase) retryIndividually(batch *pgx.Batch) {
	conn := db.GetConnection()
	for _, query := range batch.QueuedQueries {
		ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
		_, err := conn.Exec(ctx, query.SQL, query.Arguments...)
		cancel()
		if err == nil {
			continue
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (strings.HasPrefix(pgErr.Code, "22") || strings.HasPrefix(pgErr.Code, "23")) {
			logger.GetLogger().Error(
				"Dropping a query that permanently fails with a data/integrity error, it would otherwise block every other queued query forever",
				zap.String("sql", query.SQL),
				zap.String("sqlstate", pgErr.Code),
				zap.Error(err),
			)
			continue
		}

		logger.GetLogger().Error(
			"Query failed even individually, reinserting it into the queue",
			zap.Error(err),
		)
		db.batchMutex.Lock()
		db.batch.QueuedQueries = append(db.batch.QueuedQueries, query)
		db.batchMutex.Unlock()
	}
}

func (db *PostgresqlDatabase) copyBatch() *pgx.Batch {
	db.batchMutex.Lock()
	defer db.batchMutex.Unlock()

	if db.batch.Len() < 1 {
		return nil
	}

	db.flushesCounter.Inc()
	queuedQueries := make([]*pgx.QueuedQuery, db.batch.Len())
	copy(queuedQueries, db.batch.QueuedQueries)
	db.batch.QueuedQueries = db.batch.QueuedQueries[:0]
	db.batchSizeGauge.Set(0)

	batchToFlush := &pgx.Batch{QueuedQueries: queuedQueries}
	return batchToFlush
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
