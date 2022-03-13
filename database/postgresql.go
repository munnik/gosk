package database

import (
	"context"
	"embed"
	"fmt"
	"strconv"
	"sync"

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
	rawInsertQuery    = `INSERT INTO "raw_data" ("time", "collector", "value", "uuid", "type") VALUES ($1, $2, $3, $4, $5)`
	mappedInsertQuery = `INSERT INTO "mapped_data" ("time", "collector", "type", "context", "path", "value", "uuid", "origin") VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	mappedSelectQuery = `SELECT "time", "collector", "type", "context", "path", "value", "uuid", "origin" FROM "mapped_data"`
)

//go:embed migrations/*.sql
var fs embed.FS

type PostgresqlDatabase struct {
	url        string
	connection *pgxpool.Pool
	mu         sync.Mutex
}

func NewPostgresqlDatabase(c *config.PostgresqlConfig) *PostgresqlDatabase {
	return &PostgresqlDatabase{url: c.URLString}
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
	for _, err := db.GetConnection().Exec(context.Background(), rawInsertQuery, raw.Timestamp, raw.Collector, raw.Value, raw.Uuid, raw.Type); err != nil; {
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
}

func (db *PostgresqlDatabase) WriteMapped(mapped message.Mapped) {
	for _, m := range mapped.ToSingleValueMapped() {
		if str, ok := m.Value.(string); ok {
			m.Value = strconv.Quote(str)
		}
		for _, err := db.GetConnection().Exec(context.Background(), mappedInsertQuery, m.Timestamp, m.Source.Label, m.Source.Type, m.Context, m.Path, m.Value, m.Uuid, m.Origin); err != nil; {
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
				zap.String("UUID", m.Uuid.String()),
				zap.String("Origin", m.Origin),
			)
		}
	}
}

func (db *PostgresqlDatabase) ReadMapped(appendToQuery string, arguments ...interface{}) ([]message.Mapped, error) {
	rows, err := db.GetConnection().Query(context.Background(), fmt.Sprintf("%s %s", mappedSelectQuery, appendToQuery), arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]message.Mapped, 0)
	for rows.Next() {
		m := message.NewSingleValueMapped()
		rows.Scan(
			m.Timestamp,
			m.Source.Label,
			m.Source.Type,
			m.Context,
			m.Path,
			m.Value,
			m.Uuid,
			m.Origin,
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
