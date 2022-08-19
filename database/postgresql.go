package database

import (
	"context"
	"embed"
	"fmt"
	"strconv"
	"sync"
	"time"

	cache "github.com/Code-Hex/go-generics-cache"
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
	rawInsertQuery                 = `INSERT INTO "raw_data" ("time", "collector", "value", "uuid", "type") VALUES ($1, $2, $3, $4, $5)`
	selectRawQuery                 = `SELECT "time", "collector", "value", "uuid", "type" FROM "raw_data"`
	mappedInsertQuery              = `INSERT INTO "mapped_data" ("time", "collector", "type", "context", "path", "value", "uuid", "origin") VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
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
	url         string
	connection  *pgxpool.Pool
	mu          sync.Mutex
	rawCache    *cache.Cache[time.Time, []message.Raw]
	mappedCache *cache.Cache[time.Time, []message.SingleValueMapped]
}

func NewPostgresqlDatabase(c *config.PostgresqlConfig) *PostgresqlDatabase {
	return &PostgresqlDatabase{
		url:         c.URLString,
		rawCache:    cache.New[time.Time, []message.Raw](),
		mappedCache: cache.New[time.Time, []message.SingleValueMapped](),
	}
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
	// start a transaction
	transaction, err := db.GetConnection().Begin(context.Background())
	if err != nil {
		logger.GetLogger().Warn(
			"Error starting a transaction",
			zap.String("Error", err.Error()),
		)
		return
	}

	// check if timestamp is already in the cache, if not retrieve all existing rows from the database and fill the cache
	if _, ok := db.mappedCache.Get(raw.Timestamp); !ok {
		// create an empty list for the timestamp
		db.rawCache.Set(raw.Timestamp, []message.Raw{})
		rows, err := transaction.Query(context.Background(), selectRawQuery+` WHERE "time" = $1`, raw.Timestamp)
		if err != nil {
			transaction.Rollback(context.Background())
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
			transaction.Rollback(context.Background())
			continue
		}
	}

	// value is not in cache, insert into the database and add to the cache
	_, err = transaction.Exec(context.Background(), rawInsertQuery, raw.Timestamp, raw.Collector, raw.Value, raw.Uuid, raw.Type)
	if err != nil {
		logger.GetLogger().Warn(
			"Error on inserting the received data in the database",
			zap.String("Error", err.Error()),
			zap.String("Query", rawInsertQuery),
			zap.Time("Timestamp", raw.Timestamp),
			zap.String("Collector", raw.Collector),
			zap.ByteString("Value", raw.Value),
			zap.String("UUID", raw.Uuid.String()),
			zap.String("Type", raw.Type),
		)
	}
	err = transaction.Commit(context.Background())
	if err != nil {
		logger.GetLogger().Warn(
			"Error on inserting the received data in the database",
			zap.String("Error", err.Error()),
			zap.String("Query", rawInsertQuery),
			zap.Time("Timestamp", raw.Timestamp),
			zap.String("Collector", raw.Collector),
			zap.ByteString("Value", raw.Value),
			zap.String("UUID", raw.Uuid.String()),
			zap.String("Type", raw.Type),
		)
	} else {
		db.rawCache.Set(raw.Timestamp, append(cached, raw))
	}
}

func (db *PostgresqlDatabase) WriteMapped(mapped message.Mapped) {
	for _, m := range mapped.ToSingleValueMapped() {
		if str, ok := m.Value.(string); ok {
			m.Value = strconv.Quote(str)
		}

		// start a transaction
		transaction, err := db.GetConnection().Begin(context.Background())
		if err != nil {
			logger.GetLogger().Warn(
				"Error starting a transaction",
				zap.String("Error", err.Error()),
			)
			continue
		}

		// check if timestamp is already in the cache, if not retrieve all existing rows from the database and fill the cache
		if _, ok := db.mappedCache.Get(m.Timestamp); !ok {
			// create an empty list for the timestamp
			db.mappedCache.Set(m.Timestamp, []message.SingleValueMapped{})

			rows, err := transaction.Query(context.Background(), selectMappedQuery+` WHERE "time" = $1`, m.Timestamp)
			if err != nil {
				transaction.Rollback(context.Background())
				continue
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

				cached, _ := db.mappedCache.Get(m.Timestamp)
				db.mappedCache.Set(m.Timestamp, append(cached, *inDatabase))
			}
		}

		// now check the cache to see if the value is already in the cache, if so continue
		cached, _ := db.mappedCache.Get(m.Timestamp)
		for _, c := range cached {
			if c.Equals(m) {
				transaction.Rollback(context.Background())
				continue
			}
		}

		// value is not in cache, insert into the database and add to the cache
		_, err = transaction.Exec(context.Background(), mappedInsertQuery, m.Timestamp, m.Source.Label, m.Source.Type, m.Context, m.Path, m.Value, m.Source.Uuid, m.Origin)
		if err != nil {
			logger.GetLogger().Warn(
				"Error on inserting the received data in the database",
				zap.String("Error", err.Error()),
				zap.String("Query", mappedInsertQuery),
				zap.Time("Timestamp", m.Timestamp),
				zap.String("Label", m.Source.Label),
				zap.String("Type", m.Source.Type),
				zap.String("Context", m.Context),
				zap.String("Path", m.Path),
				zap.Any("Value", m.Value),
				zap.String("UUID", m.Source.Uuid.String()),
				zap.String("Origin", m.Origin),
			)
		}
		err = transaction.Commit(context.Background())
		if err != nil {
			logger.GetLogger().Warn(
				"Error on inserting the received data in the database",
				zap.String("Error", err.Error()),
				zap.String("Query", mappedInsertQuery),
				zap.Time("Timestamp", m.Timestamp),
				zap.String("Label", m.Source.Label),
				zap.String("Type", m.Source.Type),
				zap.String("Context", m.Context),
				zap.String("Path", m.Path),
				zap.Any("Value", m.Value),
				zap.String("UUID", m.Source.Uuid.String()),
				zap.String("Origin", m.Origin),
			)
		} else {
			db.mappedCache.Set(m.Timestamp, append(cached, m))
		}
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
