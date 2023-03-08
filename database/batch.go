package database

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

type Batch struct {
	rowsLock      sync.Mutex
	rows          [][]interface{}
	maxLength     int
	flushInterval time.Duration
	db            *PostgresqlDatabase
	table         pgx.Identifier
	columns       []string
}

func NewBatch(maxLength int, flushInterval time.Duration, db *PostgresqlDatabase, table pgx.Identifier, columns []string) *Batch {
	result := &Batch{
		rows:          make([][]interface{}, 0, maxLength),
		maxLength:     maxLength,
		flushInterval: flushInterval,
		db:            db,
		table:         table,
		columns:       columns,
	}
	go func() {
		ticker := time.NewTicker(flushInterval)
		for {
			<-ticker.C
			result.Tick()
		}
	}()
	return result
}

func (b *Batch) Tick() {
	b.rowsLock.Lock()
	defer b.rowsLock.Unlock()

	b.flush()
}

func (b *Batch) Append(row ...interface{}) {
	b.rowsLock.Lock()
	defer b.rowsLock.Unlock()

	b.rows = append(b.rows, row)
	if len(b.rows) >= b.maxLength {
		b.flush()
	}
}

func (b *Batch) flush() {
	if len(b.rows) == 0 {
		return
	}

	b.db.flushes.Inc()

	ctx, cancel := context.WithTimeout(context.Background(), b.db.databaseTimeout)
	defer cancel()

	inserted, err := b.db.GetConnection().CopyFrom(ctx, b.table, b.columns, pgx.CopyFromRows(b.rows))
	if err != nil {
		if ctx.Err() != nil {
			b.db.timeouts.Inc()
		}
		logger.GetLogger().Error(
			"Unable to flush batch",
			zap.Strings("Table", b.table),
			zap.Strings("Columns", b.columns),
			zap.Int("Rows expected to insert", len(b.rows)),
			zap.String("Error", err.Error()),
		)
	} else {
		logger.GetLogger().Info(
			"Batch flushed",
			zap.Strings("Table", b.table),
			zap.Strings("Columns", b.columns),
			zap.Int64("Rows inserted", inserted),
			zap.Int("Rows expected to insert", len(b.rows)),
		)
	}
	b.rows = make([][]interface{}, 0, b.maxLength)
}
