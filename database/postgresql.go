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
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/log/zapadapter"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

const (
	selectRawQuery                 = `SELECT "time", "collector", "value", "uuid", "type" FROM "raw_data"`
	selectMappedQuery              = `SELECT "time", "collector", "type", "context", "path", "value", "uuid", "origin" FROM "mapped_data"`
	selectMappedDataPointsQuery    = `SELECT count(*) FROM "mapped_data" WHERE "time" BETWEEN $1 AND $2`
	selectCompletePeriodsQuery     = `SELECT "start" FROM "remote_data" WHERE "origin" = $1 AND "local" >= "remote"`
	selectIncompletePeriodsQuery   = `SELECT "start" FROM "remote_data" WHERE "origin" = $1 AND "local" < "remote"`
	insertOrUpdatePeriodQuery      = `INSERT INTO "remote_data" ("origin", "start", "end", "remote") VALUES ($1, $2, $3, $4) ON CONFLICT ("origin", "start", "end") DO UPDATE SET "remote" = $4`
	updateLocalDataPoints          = `UPDATE "remote_data" SET "local" = $4 WHERE "origin" = $1 AND "start" = $2 AND "end" = $3`
	selectLocalAndRemoteDataPoints = `SELECT count(mapped_data.*) AS "local", "remote" FROM "mapped_data", "remote_data" WHERE "remote_data"."origin" = $1 AND "start" = $2 AND "end" = $3 AND "mapped_data"."origin" = $1 AND "mapped_data"."time" BETWEEN $2 AND $3 GROUP BY "remote";`
)

//go:embed migrations/*.sql
var fs embed.FS

type PostgresqlDatabase struct {
	url          string
	connection   *pgxpool.Pool
	mu           sync.Mutex
	rawCache     *cache.Cache[time.Time, []message.Raw]
	mappedCache  *cache.Cache[time.Time, []message.SingleValueMapped]
	rawBuffer    PostgresqlBuffer
	mappedBuffer PostgresqlBuffer
}

func NewPostgresqlDatabase(c *config.PostgresqlConfig) *PostgresqlDatabase {
	result := &PostgresqlDatabase{
		url:         c.URLString,
		rawCache:    cache.New(cache.AsFIFO[time.Time, []message.Raw](fifo.WithCapacity(20 * 1024))),
		mappedCache: cache.New(cache.AsFIFO[time.Time, []message.SingleValueMapped](fifo.WithCapacity(20 * 1024))),
	}
	result.rawBuffer = *NewPostgresqlBuffer(result.GetConnection(), "raw_data", []string{"time", "collector", "value", "uuid", "type"}, c.BufferSize, c.BufferFlushInterval)
	result.mappedBuffer = *NewPostgresqlBuffer(result.GetConnection(), "mapped_data", []string{"time", "collector", "type", "context", "path", "value", "uuid", "origin"}, c.BufferSize, c.BufferFlushInterval)
	return result
}

func (db *PostgresqlDatabase) GetConnection() *pgxpool.Pool {
	// check if a connection exist and is pingable, return the connection on success
	if db.connection != nil {
		if err := db.connection.Ping(context.Background()); err == nil {
			return db.connection
		}
	}

	db.mu.Lock()
	defer db.mu.Unlock()

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
	if _, ok := db.mappedCache.Get(raw.Timestamp); !ok {
		// create an empty list for the timestamp
		db.rawCache.Set(raw.Timestamp, []message.Raw{})
		rows, err := db.GetConnection().Query(context.Background(), selectRawQuery+` WHERE "time" = $1`, raw.Timestamp)
		if err != nil {
			return
		}
		defer rows.Close()

		for rows.Next() {
			inDatabase := message.NewRaw()
			rows.Scan(
				&inDatabase.Timestamp,
				&inDatabase.Collector,
				&inDatabase.Value,
				&inDatabase.Uuid,
				&inDatabase.Type,
			)

			cached, _ := db.rawCache.Get(raw.Timestamp)
			db.rawCache.Set(raw.Timestamp, append(cached, *inDatabase))
		}
	}

	// now check the cache to see if the value is already in the cache, if so continue
	cached, _ := db.rawCache.Get(raw.Timestamp)
	for _, c := range cached {
		if c.Equals(raw) {
			return
		}
	}

	// value is not in cache, insert into the database and add to the cache
	db.rawCache.Set(raw.Timestamp, append(cached, raw))
	// `INSERT INTO "raw_data" ("time", "collector", "value", "uuid", "type") VALUES ($1, $2, $3, $4, $5)`
	db.rawBuffer.Add([]interface{}{
		raw.Timestamp,
		raw.Collector,
		raw.Value,
		raw.Uuid,
		raw.Type,
	})
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
	if _, ok := db.mappedCache.Get(svm.Timestamp); !ok {
		// create an empty list for the timestamp
		db.mappedCache.Set(svm.Timestamp, []message.SingleValueMapped{})

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
			)

			cached, _ := db.mappedCache.Get(svm.Timestamp)
			db.mappedCache.Set(svm.Timestamp, append(cached, *inDatabase))
		}
	}

	// now check the cache to see if the value is already in the cache, if so continue
	cached, _ := db.mappedCache.Get(svm.Timestamp)
	for _, c := range cached {
		if c.Equals(svm) {
			return
		}
	}

	// value is not in cache, insert into the database and add to the cache
	db.mappedCache.Set(svm.Timestamp, append(cached, svm))

	// `INSERT INTO "mapped_data" ("time", "collector", "type", "context", "path", "value", "uuid", "origin") VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	db.mappedBuffer.Add([]interface{}{
		svm.Timestamp,
		svm.Source.Label,
		svm.Source.Type,
		svm.Context,
		svm.Path,
		svm.Value,
		svm.Source.Uuid,
		svm.Origin,
	})
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
func (db *PostgresqlDatabase) SelectCompletePeriods(origin string) (map[time.Time]struct{}, error) {
	return db.selectPeriods(selectCompletePeriodsQuery, origin)
}

// Returns the start timestamp of each period that has the same or more local rows than remote
func (db *PostgresqlDatabase) SelectIncompletePeriods(origin string) (map[time.Time]struct{}, error) {
	return db.selectPeriods(selectIncompletePeriodsQuery, origin)
}

// Returns the start timestamp of each period that has the same or more local rows than remote
func (db *PostgresqlDatabase) selectPeriods(query string, origin string) (map[time.Time]struct{}, error) {
	result := map[time.Time]struct{}{}
	rows, err := db.GetConnection().Query(context.Background(), query, origin)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	var completedPeriod time.Time
	for rows.Next() {
		rows.Scan(&completedPeriod)
		result[completedPeriod] = struct{}{}
	}
	return result, nil
}

// Creates a period in the database, the default value for the local data points is -1
func (db *PostgresqlDatabase) CreatePeriod(origin string, start time.Time, end time.Time, remote int) error {
	_, err := db.GetConnection().Exec(context.Background(), insertOrUpdatePeriodQuery, origin, start, end, remote)
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

type PostgresqlBuffer struct {
	lastFlush   time.Time
	mu          sync.Mutex
	pool        *pgxpool.Pool
	table       pgx.Identifier
	columnNames []string
	size        int
	rows        [][]interface{}
}

func NewPostgresqlBuffer(pool *pgxpool.Pool, table string, columnNames []string, size int, flushInterval int) *PostgresqlBuffer {
	result := &PostgresqlBuffer{
		lastFlush: time.Now(),
		pool:      pool,
		table:     pgx.Identifier{table},
		size:      size,
		rows:      make([][]interface{}, size+1),
	}
	go func() {
		ticker := time.NewTicker(time.Second * time.Duration(flushInterval))
		for {
			<-ticker.C
			if time.Now().Add(-time.Second * time.Duration(flushInterval)).Before(result.lastFlush) {
				result.flush()
			}
		}
	}()

	return result
}

func (b *PostgresqlBuffer) Add(row []interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.rows = append(b.rows, row)
	if len(b.rows) > b.size {
		b.flush()
	}
}

func (b *PostgresqlBuffer) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()

	src := pgx.CopyFromRows(b.rows)
	b.pool.CopyFrom(context.Background(), b.table, b.columnNames, src)
	if src.Err() != nil {
		logger.GetLogger().Error(
			"CopyFrom failed",
			zap.String("Error", src.Err().Error()),
		)
	}
	b.rows = make([][]interface{}, b.size+1)
	b.lastFlush = time.Now()
}
