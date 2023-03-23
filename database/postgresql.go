package database

import (
	"context"
	"embed"
	"fmt"
	"strconv"
	"sync"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/Code-Hex/go-generics-cache/policy/fifo"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/log/zapadapter"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

const (
	rawInsertQuery                 = `INSERT INTO "raw_data" ("time", "connector", "value", "raw_uuid", "type") VALUES ($1, $2, $3, $4, $5)`
	selectRawQuery                 = `SELECT "time", "connector", "value", "raw_uuid", "type" FROM "raw_data"`
	mappedInsertQuery              = `INSERT INTO "mapped_data" ("time", "connector", "type", "context", "path", "value", "raw_uuid", "origin", "transfer_uuid") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	selectMappedQuery              = `SELECT "time", "connector", "type", "context", "path", "value", "raw_uuid", "origin", "transfer_uuid" FROM "mapped_data"`
	selectLocalCountQuery          = `SELECT "count" FROM "transfer_local_data" WHERE "origin" = $1 AND "start" = $2`
	selectExistingRemoteCounts     = `SELECT "origin", "start" FROM "transfer_remote_data" WHERE "start" >= $1`
	selectIncompletePeriodsQuery   = `SELECT "origin", "start" FROM "transfer_data" WHERE "local_count" < "remote_count" * $1`
	insertOrUpdateRemoteData       = `INSERT INTO "transfer_remote_data" ("start", "origin", "count") VALUES ($1, $2, $3) ON CONFLICT ("start", "origin") DO UPDATE SET "count" = $3`
	logTransferInsertQuery         = `INSERT INTO "transfer_log" ("time", "origin", "message") VALUES (NOW(), $1, $2)`
	selectMappedCountPerUuid       = `SELECT "raw_uuid", COUNT("raw_uuid") FROM "mapped_data" WHERE "origin" = $1 AND "time" BETWEEN $2 AND $2 + '5m'::interval GROUP BY 1`
	selectFirstMappedDataPerOrigin = `SELECT "origin", MIN("start") FROM "transfer_local_data" GROUP BY 1`
)

//go:embed migrations/*.sql
var fs embed.FS

const databaseTimeout time.Duration = 1 * time.Second

type PostgresqlDatabase struct {
	url             string
	connection      *pgxpool.Pool
	connectionMutex sync.Mutex
	rawCache        *cache.Cache[int64, []message.Raw]
	mappedCache     *cache.Cache[int64, []message.SingleValueMapped]
	batch           *pgx.Batch
	batchSize       int
	lastFlush       time.Time
	flushMutex      sync.Mutex
	completeRatio   float64
	upgradeDone     bool
	flushes         prometheus.Counter
	lastFlushGauge  prometheus.Gauge
	writes          prometheus.Counter
	timeouts        prometheus.Counter
	batchSizeGauge  prometheus.Gauge
	cacheMisses     prometheus.Counter
}

func NewPostgresqlDatabase(c *config.PostgresqlConfig) *PostgresqlDatabase {
	result := &PostgresqlDatabase{
		url:            c.URLString,
		rawCache:       cache.New(cache.AsFIFO[int64, []message.Raw](fifo.WithCapacity(20 * 1024))),
		mappedCache:    cache.New(cache.AsFIFO[int64, []message.SingleValueMapped](fifo.WithCapacity(20 * 1024))),
		batchSize:      c.BatchSize,
		completeRatio:  c.CompleteRatio,
		batch:          &pgx.Batch{},
		lastFlush:      time.Now(),
		upgradeDone:    false,
		flushes:        promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_flushes_total", Help: "total number batches flushed"}),
		lastFlushGauge: promauto.NewGauge(prometheus.GaugeOpts{Name: "gosk_psql_last_flush_time", Help: "last db flush"}),
		writes:         promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_writes_total", Help: "total number of deltas added to queue"}),
		timeouts:       promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_timeouts_total", Help: "total number timeouts"}),
		batchSizeGauge: promauto.NewGauge(prometheus.GaugeOpts{Name: "gosk_psql_batch_length", Help: "number of deltas in current batch"}),
		cacheMisses:    promauto.NewCounter(prometheus.CounterOpts{Name: "gosk_psql_cache_misses_total", Help: "total number of cache misses"}),
	}
	go func() {
		ticker := time.NewTicker(time.Second * time.Duration(c.BatchFlushInterval))
		for {
			<-ticker.C
			if time.Now().After(result.lastFlush.Add(time.Second * time.Duration(c.BatchFlushInterval))) {
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
	conf.LazyConnect = true
	conf.ConnConfig.Logger = zapadapter.NewLogger(logger.GetLogger())
	conf.ConnConfig.LogLevel = pgx.LogLevelWarn

	conn, err := pgxpool.ConnectConfig(context.Background(), conf)
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

func (db *PostgresqlDatabase) WriteRaw(raw message.Raw) {
	db.writes.Inc()
	// check if timestamp is already in the cache, if not retrieve all existing rows from the database and fill the cache
	if _, ok := db.mappedCache.Get(raw.Timestamp.UnixMicro()); !ok {
		db.cacheMisses.Inc()
		// create an empty list for the timestamp
		db.rawCache.Set(raw.Timestamp.UnixMicro(), []message.Raw{})
		ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
		defer cancel()
		rows, err := db.GetConnection().Query(ctx, selectRawQuery+` WHERE "time" = $1`, raw.Timestamp)
		if err != nil {
			return
		} else if ctx.Err() != nil {
			logger.GetLogger().Error("Timeout during cache lookup")
			db.timeouts.Inc()
			return
		}
		defer rows.Close()

		for rows.Next() {
			inDatabase := message.NewRaw()
			rows.Scan(
				&inDatabase.Timestamp,
				&inDatabase.Connector,
				&inDatabase.Value,
				&inDatabase.Uuid,
				&inDatabase.Type,
			)

			cached, _ := db.rawCache.Get(raw.Timestamp.UnixMicro())
			db.rawCache.Set(raw.Timestamp.UnixMicro(), append(cached, *inDatabase))
		}
	}

	// now check the cache to see if the value is already in the cache, if so continue
	cached, _ := db.rawCache.Get(raw.Timestamp.UnixMicro())
	for _, c := range cached {
		if c.Equals(raw) {
			return
		}
	}

	// value is not in cache, insert into the database and add to the cache
	db.rawCache.Set(raw.Timestamp.UnixMicro(), append(cached, raw))
	db.batch.Queue(rawInsertQuery, raw.Timestamp, raw.Connector, raw.Value, raw.Uuid, raw.Type)
	db.batchSizeGauge.Inc()
	if db.batch.Len() > db.batchSize {
		go db.flushBatch()
	}
}

func (db *PostgresqlDatabase) WriteMapped(mapped message.Mapped) {
	for _, svm := range mapped.ToSingleValueMapped() {
		db.WriteSingleValueMapped(svm)
	}
}

func (db *PostgresqlDatabase) WriteSingleValueMapped(svm message.SingleValueMapped) {
	db.writes.Inc()
	if str, ok := svm.Value.(string); ok {
		svm.Value = strconv.Quote(str)
	}
	// check if timestamp is already in the cache, if not retrieve all existing rows from the database and fill the cache
	if _, ok := db.mappedCache.Get(svm.Timestamp.UnixMicro()); !ok {
		db.cacheMisses.Inc()
		// create an empty list for the timestamp
		db.mappedCache.Set(svm.Timestamp.UnixMicro(), []message.SingleValueMapped{})
		ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
		defer cancel()
		rows, err := db.GetConnection().Query(ctx, selectMappedQuery+` WHERE "time" = $1`, svm.Timestamp)
		if err != nil {
			return
		} else if ctx.Err() != nil {
			logger.GetLogger().Error("Timeout during cache lookup")
			db.timeouts.Inc()
			return
		}
		defer rows.Close()

		for rows.Next() {
			inDatabase := message.NewSingleValueMapped()
			rows.Scan(
				&inDatabase.Timestamp,
				&inDatabase.Source.Label,
				&inDatabase.Source.Type,
				&inDatabase.Context,
				&inDatabase.Path,
				&inDatabase.Value,
				&inDatabase.Source.Uuid,
				&inDatabase.Origin,
				&inDatabase.Source.TransferUuid,
			)

			cached, _ := db.mappedCache.Get(svm.Timestamp.UnixMicro())
			db.mappedCache.Set(svm.Timestamp.UnixMicro(), append(cached, *inDatabase))
		}
	}

	// now check the cache to see if the value is already in the cache, if so continue
	cached, _ := db.mappedCache.Get(svm.Timestamp.UnixMicro())
	for _, c := range cached {
		if c.Equals(svm) {
			return
		}
	}
	// value is not in cache, insert into the database and add to the cache
	db.batchSizeGauge.Inc()
	db.mappedCache.Set(svm.Timestamp.UnixMicro(), append(cached, svm))
	db.batch.Queue(mappedInsertQuery, svm.Timestamp, svm.Source.Label, svm.Source.Type, svm.Context, svm.Path, svm.Value, svm.Source.Uuid, svm.Origin, svm.Source.TransferUuid)
	if db.batch.Len() > db.batchSize {
		go db.flushBatch()
	}
}

func (db *PostgresqlDatabase) ReadMapped(appendToQuery string, arguments ...interface{}) ([]message.Mapped, error) {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, fmt.Sprintf("%s %s", selectMappedQuery, appendToQuery), arguments...)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeouts.Inc()
		return nil, ctx.Err()
	}
	defer rows.Close()

	result := make([]message.Mapped, 0)
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
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectFirstMappedDataPerOrigin)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeouts.Inc()
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
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectExistingRemoteCounts, from)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeouts.Inc()
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
func (db *PostgresqlDatabase) SelectIncompletePeriods() (map[string][]time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectIncompletePeriodsQuery, db.completeRatio)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeouts.Inc()
		return nil, ctx.Err()
	}
	defer rows.Close()

	result := make(map[string][]time.Time)
	var origin string
	var start time.Time
	for rows.Next() {
		err := rows.Scan(&origin, &start)
		if err != nil {
			return nil, err
		}
		if _, ok := result[origin]; !ok {
			result[origin] = make([]time.Time, 0)
		}
		result[origin] = append(result[origin], start)
	}
	// check for errors after last call to .Next()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (db *PostgresqlDatabase) SelectCountMapped(origin string, start time.Time) (int, error) {
	var result int
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	err := db.GetConnection().QueryRow(ctx, selectLocalCountQuery, origin, start).Scan(&result)
	if err != nil {
		return 0, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeouts.Inc()
		return 0, ctx.Err()
	}

	return result, nil
}

// Return the number of rows per (raw) uuid in the mapped_data table
func (db *PostgresqlDatabase) SelectCountPerUuid(origin string, start time.Time) (map[uuid.UUID]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, selectMappedCountPerUuid, origin, start)
	if err != nil {
		return nil, err
	} else if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database lookup")
		db.timeouts.Inc()
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
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	_, err := db.GetConnection().Exec(ctx, insertOrUpdateRemoteData, start, origin, count)
	if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database insertion")
		db.timeouts.Inc()
		return ctx.Err()
	}
	return err
}

// Log the transfer request
func (db *PostgresqlDatabase) LogTransferRequest(origin string, message interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	_, err := db.GetConnection().Exec(ctx, logTransferInsertQuery, origin, message)
	if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database insertion")
		db.timeouts.Inc()
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
	db.flushMutex.Lock()
	db.flushes.Inc()
	batchPtr := db.batch
	db.batch = &pgx.Batch{}
	db.batchSizeGauge.Set(0)
	defer db.flushMutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), databaseTimeout)
	defer cancel()
	result := db.GetConnection().SendBatch(ctx, batchPtr)
	// todo, determine if inserts went well
	if ctx.Err() != nil {
		logger.GetLogger().Error("Timeout during database insertion")
		db.timeouts.Inc()
		return
	}
	if err := result.Close(); err != nil {
		logger.GetLogger().Error(
			"Unable to flush batch",
			zap.String("Error", err.Error()),
		)
		return
	}
	db.lastFlush = time.Now()
	db.lastFlushGauge.SetToCurrentTime()
}
