package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strconv"
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

const (
	rawInsertQuery                 = `INSERT INTO "raw_data" ("time", "connector", "value", "uuid", "type") VALUES ($1, $2, $3, $4, $5)`
	selectRawQuery                 = `SELECT "time", "connector", "value", "uuid", "type" FROM "raw_data"`
	mappedInsertQuery              = `INSERT INTO "%s" ("time", "connector", "type", "context", "path", "value", "uuid", "origin", "transfer_uuid") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT ("time", "origin", "context", "connector", "path") DO UPDATE SET value = EXCLUDED.value`
	selectMappedQuery              = `SELECT "time", "connector", "type", "context", "path", "value", "uuid", "origin", "transfer_uuid" FROM "mapped_data"`
	selectMostRecentMappedQuery    = `SELECT DISTINCT ON ("context", "path") "time", "connector", "type", "context", "path", "value", "uuid", "origin", "transfer_uuid" FROM "mapped_data" WHERE "time" > $1 ORDER BY "context", "path", "time" DESC`
	selectLocalCountQuery          = `SELECT "count" FROM "transfer_local_data" WHERE "origin" = $1 AND "start" = $2`
	selectExistingRemoteCounts     = `SELECT "origin", "start" FROM "transfer_remote_data" WHERE "start" >= $1`
	selectIncompletePeriodsQuery   = `SELECT "origin", "start", "local_count", "remote_count" FROM "transfer_data" WHERE "local_count" < "remote_count" * $1::double precision ORDER BY "start" DESC`
	insertOrUpdateRemoteData       = `INSERT INTO "transfer_remote_data" ("start", "origin", "count") VALUES ($1, $2, $3) ON CONFLICT ("start", "origin") DO UPDATE SET "count" = EXCLUDED.count`
	logTransferInsertQuery         = `INSERT INTO "transfer_log" ("time", "origin", "message") VALUES (NOW(), $1, $2)`
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

type PostgresqlDatabase struct {
	url             string
	connection      *pgxpool.Pool
	connectionMutex sync.Mutex
	batch           *pgx.Batch
	batchSize       int
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
				result.flushBatch()
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

func (db *PostgresqlDatabase) WriteRaw(raw *message.Raw) {
	db.writesCounter.Inc()
	db.batchMutex.Lock()
	db.batch.Queue(rawInsertQuery, raw.Timestamp, raw.Connector, raw.Value, raw.Uuid, raw.Type).Exec(func(ct pgconn.CommandTag) error {
		if ct.RowsAffected() == 0 {
			logger.GetLogger().Warn("0 rows affected",
				zap.String("query", rawInsertQuery),
				zap.Time("timestamp", raw.Timestamp),
				zap.String("connector", raw.Connector),
				zap.ByteString("value", raw.Value),
				zap.String("uuid", raw.Uuid.String()),
				zap.String("type", raw.Type),
			)
		}

		return nil
	})
	db.batchMutex.Unlock()

	db.batchSizeGauge.Inc()
	if db.batch.Len() > db.batchSize {
		go db.flushBatch()
	}
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

	if str, ok := svm.Value.(string); ok {
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
	db.batchMutex.Unlock()

	db.batchSizeGauge.Inc()
	if db.batch.Len() > db.batchSize {
		go db.flushBatch()
	}
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

	query := fmt.Sprintf(`INSERT INTO "gosk"."static_data" ("context", "%s") VALUES ($1, $2) ON CONFLICT("context") DO UPDATE SET "%s" = EXCLUDED."%s";`, column, column, column)
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
	db.batchMutex.Unlock()

	db.batchSizeGauge.Inc()
	if db.batch.Len() > db.batchSize {
		go db.flushBatch()
	}
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

// Log the transfer request
func (db *PostgresqlDatabase) LogTransferRequest(origin string, message interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	_, err := db.GetConnection().Exec(ctx, logTransferInsertQuery, origin, message)
	if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database insertion")
		db.timeoutsCounter.Inc()
		return ctx.Err()
	}
	return err
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

func (db *PostgresqlDatabase) flushBatch() {
	// db.flushMutex.Lock() // make sure only one flush runs at the same time, to prevent deadlocks
	// defer db.flushMutex.Unlock()

	batchToFlush := db.copyBatch()
	// prevent flushing when queue is empty
	if batchToFlush == nil {
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
	result := db.GetConnection().SendBatch(ctx, batchToFlush)

	if err := result.Close(); err != nil {
		if ctx.Err() != nil {
			logger.GetLogger().Error("Timeout during database insertion", zap.Error(ctx.Err()), zap.Error(err))
			db.timeoutsCounter.Inc()
			return
		}
		logger.GetLogger().Error(
			"Unable to flush batch, reinserting queries to the queue",
			zap.String("Error", err.Error()),
		)
		for _, query := range batchToFlush.QueuedQueries {
			db.batchMutex.Lock()
			db.batch.QueuedQueries = append(db.batch.QueuedQueries, query)
			db.batchMutex.Unlock()
			time.Sleep(10 * time.Second / time.Duration(batchToFlush.Len())) // spread out the queries over 10 seconds
		}
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
