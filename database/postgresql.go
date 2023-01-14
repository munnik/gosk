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
	"go.uber.org/zap"
)

const (
	rawInsertQuery    = `INSERT INTO "raw_data" ("time", "connector", "value", "uuid", "type") VALUES ($1, $2, $3, $4, $5)`
	selectRawQuery    = `SELECT "time", "connector", "value", "uuid", "type" FROM "raw_data"`
	mappedInsertQuery = `INSERT INTO "mapped_data" ("time", "connector", "type", "context", "path", "value", "uuid", "origin", "transfer_uuid") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	// on conflict(time, context, path, value, uuid, origin) do nothing
	selectMappedQuery              = `SELECT "time", "connector", "type", "context", "path", "value", "uuid", "origin", "transfer_uuid" FROM "mapped_data"`
	selectMappedDataPointsQuery    = `SELECT count(*) FROM "mapped_data" WHERE "time" BETWEEN $1 AND $2`
	selectCompletePeriodsQuery     = `SELECT "start" FROM "remote_data" WHERE "origin" = $1 AND "local" >= "remote" * $2`
	selectIncompletePeriodsQuery   = `SELECT "start" FROM "remote_data" WHERE "origin" = $1 AND "local" < "remote" * $2`
	insertOrUpdatePeriodQuery      = `INSERT INTO "remote_data" ("origin", "start", "end", "remote", "last_count_request") VALUES ($1, $2, $3, $4, $5) ON CONFLICT ("origin", "start", "end") DO UPDATE SET "remote" = $4, "last_count_request" = $5, "count_requests" = "remote_data"."count_requests" + 1`
	updateLocalDataPoints          = `UPDATE "remote_data" SET "local" = $4 WHERE "origin" = $1 AND "start" = $2 AND "end" = $3`
	selectLocalAndRemoteDataPoints = `select local, remote from (select "remote" FROM "remote_data" WHERE "remote_data"."origin" = $1  AND "start" = $2 AND "end" = $3) as a,( SELECT count(*) AS "local" FROM "mapped_data" WHERE "origin" = $1 AND "mapped_data"."time" BETWEEN $2 AND $3) as b;`
	logRequestQuery                = `INSERT INTO "transfer_log" ("time", "uuid", "origin", "start", "end", "local", "remote") VALUES ($1, $2, $3, $4, $5, $6, $7)`
	updateStatisticsQuery          = `UPDATE "remote_data" SET "last_data_request" = $1, "data_requests" = "data_requests" + 1 WHERE "origin" = $2 AND "start" = $3 AND "end" = $4`
)

//go:embed migrations/*.sql
var fs embed.FS

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
}

func NewPostgresqlDatabase(c *config.PostgresqlConfig) *PostgresqlDatabase {
	result := &PostgresqlDatabase{
		url:           c.URLString,
		rawCache:      cache.New(cache.AsFIFO[int64, []message.Raw](fifo.WithCapacity(20 * 1024))),
		mappedCache:   cache.New(cache.AsFIFO[int64, []message.SingleValueMapped](fifo.WithCapacity(20 * 1024))),
		batchSize:     c.BatchSize,
		completeRatio: c.CompleteRatio,
		batch:         &pgx.Batch{},
		lastFlush:     time.Now(),
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
	// check if timestamp is already in the cache, if not retrieve all existing rows from the database and fill the cache
	if _, ok := db.mappedCache.Get(raw.Timestamp.UnixMicro()); !ok {
		// create an empty list for the timestamp
		db.rawCache.Set(raw.Timestamp.UnixMicro(), []message.Raw{})
		rows, err := db.GetConnection().Query(context.Background(), selectRawQuery+` WHERE "time" = $1`, raw.Timestamp)
		if err != nil {
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
	if str, ok := svm.Value.(string); ok {
		svm.Value = strconv.Quote(str)
	}
	// check if timestamp is already in the cache, if not retrieve all existing rows from the database and fill the cache
	if _, ok := db.mappedCache.Get(svm.Timestamp.UnixMicro()); !ok {
		// create an empty list for the timestamp
		db.mappedCache.Set(svm.Timestamp.UnixMicro(), []message.SingleValueMapped{})
		rows, err := db.GetConnection().Query(context.Background(), selectMappedQuery+` WHERE "time" = $1`, svm.Timestamp)
		if err != nil {
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
	db.mappedCache.Set(svm.Timestamp.UnixMicro(), append(cached, svm))
	db.batch.Queue(mappedInsertQuery, svm.Timestamp, svm.Source.Label, svm.Source.Type, svm.Context, svm.Path, svm.Value, svm.Source.Uuid, svm.Origin, svm.Source.TransferUuid)
	if db.batch.Len() > db.batchSize {
		go db.flushBatch()
	}
}

func (db *PostgresqlDatabase) ReadMapped(appendToQuery string, arguments ...interface{}) ([]message.Mapped, error) {
	rows, err := db.GetConnection().Query(context.Background(), fmt.Sprintf("%s %s", selectMappedQuery, appendToQuery), arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]message.Mapped, 0)
	for rows.Next() {
		m := message.NewSingleValueMapped()
		rows.Scan(
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
		if m.Value, err = message.Decode(m.Value); err != nil {
			logger.GetLogger().Warn(
				"Could not decode value",
				zap.String("Error", err.Error()),
				zap.Any("Value", m.Value),
			)
		}
		result = append(result, m.ToMapped())
	}

	return result, nil
}

func (db *PostgresqlDatabase) ReadMappedCount(start time.Time, end time.Time) (int, error) {
	rows, err := db.GetConnection().Query(context.Background(), selectMappedDataPointsQuery, start, end)
	if err != nil {
		return -1, err
	}
	defer rows.Close()
	var count int
	rows.Next()
	rows.Scan(&count)

	return count, nil
}

// Returns the start timestamp of each period that has the same or more local rows than remote
func (db *PostgresqlDatabase) SelectCompletePeriods(origin string) (map[int64]time.Time, error) {
	return db.selectPeriods(selectCompletePeriodsQuery, origin, db.completeRatio)
}

// Returns the start timestamp of each period that has the same or more local rows than remote
func (db *PostgresqlDatabase) SelectIncompletePeriods(origin string) (map[int64]time.Time, error) {
	return db.selectPeriods(selectIncompletePeriodsQuery, origin, db.completeRatio)
}

// Returns the start timestamp of each period that has the same or more local rows than remote
func (db *PostgresqlDatabase) selectPeriods(query string, origin string, completeRatio float64) (map[int64]time.Time, error) {
	result := map[int64]time.Time{}
	rows, err := db.GetConnection().Query(context.Background(), query, origin, completeRatio)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	var completedPeriod time.Time
	for rows.Next() {
		rows.Scan(&completedPeriod)
		result[completedPeriod.UnixMicro()] = completedPeriod
	}
	return result, nil
}

// Creates a period in the database, the default value for the local data points is -1
func (db *PostgresqlDatabase) CreatePeriod(origin string, start time.Time, end time.Time, remote int) error {
	_, err := db.GetConnection().Exec(context.Background(), insertOrUpdatePeriodQuery, origin, start, end, remote, time.Now())
	return err
}

// Update the local data points in the database
func (db *PostgresqlDatabase) UpdateLocalDataPoints(origin string, start time.Time, end time.Time, dataPoints int) error {
	_, err := db.GetConnection().Exec(context.Background(), updateLocalDataPoints, origin, start, end, dataPoints)
	return err
}

func (db *PostgresqlDatabase) SelectLocalAndRemoteDataPoints(origin string, start time.Time, end time.Time) (int, int, error) {
	rows, err := db.GetConnection().Query(context.Background(), selectLocalAndRemoteDataPoints, origin, start, end)
	if err != nil {
		return -1, -1, err
	}
	defer rows.Close()
	var local int
	var remote int
	rows.Next()
	rows.Scan(&local, &remote)
	return local, remote, nil
}

// Log the transfer request
func (db *PostgresqlDatabase) LogTransferRequest(time time.Time, uuid uuid.UUID, origin string, start time.Time, end time.Time, local int, remote int) error {
	_, err := db.GetConnection().Exec(context.Background(), logRequestQuery, time, uuid, origin, start, end, local, remote)
	return err
}

func (db *PostgresqlDatabase) UpdateStatistics(time time.Time, origin string, start time.Time, end time.Time) error {
	_, err := db.GetConnection().Exec(context.Background(), updateStatisticsQuery, time, origin, start, end)
	return err
}

func (db *PostgresqlDatabase) UpgradeDatabase() error {
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, db.url)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
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
	batchPtr := db.batch
	db.batch = &pgx.Batch{}
	defer db.flushMutex.Unlock()
	result := db.GetConnection().SendBatch(context.Background(), batchPtr)
	// todo, determine if inserts went well

	if err := result.Close(); err != nil {
		logger.GetLogger().Error(
			"Unable to flush batch",
			zap.String("Error", err.Error()),
		)
		return
	}

	db.lastFlush = time.Now()
}
