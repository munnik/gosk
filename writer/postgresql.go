package writer

import (
	"context"
	"embed"
	"encoding/json"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var fs embed.FS

const (
	rawInsertQuery    = `INSERT INTO "raw_data" ("time", "collector", "value", "uuid", "type") VALUES ($1, $2, $3, $4, $5)`
	mappedInsertQuery = `INSERT INTO "mapped_data" ("time", "collector", "type", "context", "path", "value", "uuid", "origin") VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
)

type PostgresqlWriter struct {
	url        string
	connection *pgxpool.Pool
}

func NewPostgresqlWriter(c *config.PostgresqlConfig) *PostgresqlWriter {
	return &PostgresqlWriter{url: c.URLString}
}

func (w *PostgresqlWriter) GetConnection() *pgxpool.Pool {
	if w.connection != nil {
		return w.connection
	}

	conn, err := pgxpool.Connect(context.Background(), w.url)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the database",
			zap.String("URL", w.url),
			zap.String("Error", err.Error()),
		)
		return nil
	}
	w.connection = conn
	return conn
}

func (w *PostgresqlWriter) WriteRaw(subscriber mangos.Socket) {
	if err := w.UpgradeDatabase(); err != nil {
		logger.GetLogger().Fatal(
			"Could not update the database",
			zap.String("Error", err.Error()),
		)
		return
	}

	conn := w.GetConnection()
	defer conn.Close()

	raw := &message.Raw{}

	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, raw); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.WriteSingleRawEntry(raw)
	}
}

func (w *PostgresqlWriter) WriteSingleRawEntry(raw *message.Raw) {
	if _, err := w.GetConnection().Exec(context.Background(), rawInsertQuery, raw.Timestamp, raw.Collector, raw.Value, raw.Uuid, raw.Type); err != nil {
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

func (w *PostgresqlWriter) WriteMapped(subscriber mangos.Socket) {
	if err := w.UpgradeDatabase(); err != nil {
		logger.GetLogger().Fatal(
			"Could not update the database",
			zap.String("Error", err.Error()),
		)
		return
	}

	conn := w.GetConnection()
	defer conn.Close()

	mapped := &message.Mapped{}

	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.WriteSingleMappedEntry(mapped)
	}
}

func (w *PostgresqlWriter) WriteSingleMappedEntry(mapped *message.Mapped) {
	for _, update := range mapped.Updates {
		for _, value := range update.Values {
			if str, ok := value.Value.(string); ok {
				value.Value = strconv.Quote(str)
			}
			if _, err := w.GetConnection().Exec(context.Background(), mappedInsertQuery, update.Timestamp, update.Source.Label, update.Source.Type, mapped.Context, value.Path, value.Value, value.Uuid, mapped.Origin); err != nil {
				logger.GetLogger().Warn(
					"Error on inserting the received data in the database",
					zap.String("Error", err.Error()),
					zap.String("Query", mappedInsertQuery),
					zap.Time("Timestamp", update.Timestamp),
					zap.String("Label", update.Source.Label),
					zap.String("Type", update.Source.Type),
					zap.String("Context", mapped.Context),
					zap.String("Path", value.Path),
					zap.Any("Value", value.Value),
					zap.String("UUID", value.Uuid.String()),
					zap.String("Origin", mapped.Origin),
				)
			}
		}
	}
}

func (w *PostgresqlWriter) UpgradeDatabase() error {
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, w.url)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func (w *PostgresqlWriter) DowngradeDatabase() error {
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, w.url)
	if err != nil {
		return err
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
