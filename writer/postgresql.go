package writer

import (
	"context"
	"embed"
	"encoding/json"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v4"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

//go:embed migrations/*.sql
var fs embed.FS

type PostgresqlWriter struct {
	Url string
}

func NewPostgresqlWriter() *PostgresqlWriter {
	return &PostgresqlWriter{}
}

func (w *PostgresqlWriter) WithUrl(u string) *PostgresqlWriter {
	w.Url = u
	return w
}

func (w *PostgresqlWriter) WriteRaw(subscriber mangos.Socket) {
	if err := w.updateDatabase(); err != nil {
		logger.GetLogger().Fatal(
			"Could not update the database",
			zap.String("Error", err.Error()),
		)
		return
	}

	conn, err := pgx.Connect(context.Background(), w.Url)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the database",
			zap.String("URL", w.Url),
			zap.String("Error", err.Error()),
		)
		return
	}
	defer conn.Close(context.Background())

	raw := &message.Raw{}
	query := `INSERT INTO raw_data ("time", "collector", "value", "uuid", "type") VALUES ($1, $2, $3, $4, $5)`

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
		if _, err := conn.Exec(context.Background(), query, raw.Timestamp, raw.Collector, raw.Value, raw.Uuid, raw.Type); err != nil {
			logger.GetLogger().Warn(
				"Error on inserting the received data in the database",
				zap.String("Error", err.Error()),
			)
		}
	}
}

func (w *PostgresqlWriter) WriteMapped(subscriber mangos.Socket) {
	if err := w.updateDatabase(); err != nil {
		logger.GetLogger().Fatal(
			"Could not update the database",
			zap.String("Error", err.Error()),
		)
		return
	}

	conn, err := pgx.Connect(context.Background(), w.Url)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not connect to the database",
			zap.String("URL", w.Url),
			zap.String("Error", err.Error()),
		)
		return
	}
	defer conn.Close(context.Background())

	mapped := &message.Mapped{}
	query := `INSERT INTO mapped_data ("time", "collector", "type", "context", "path", "value", "uuid", "origin") VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

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
		for _, update := range mapped.Updates {
			for _, value := range update.Values {
				if _, err := conn.Exec(context.Background(), query, update.Timestamp, update.Source.Label, update.Source.Type, mapped.Context, value.Path, value.Value, value.Uuid, mapped.Origin); err != nil {
					logger.GetLogger().Warn(
						"Error on inserting the received data in the database",
						zap.String("Error", err.Error()),
					)
				}
			}
		}
	}
}

func (w *PostgresqlWriter) updateDatabase() error {
	d, err := iofs.New(fs, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", d, w.Url)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
