package database

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

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
		lastFlush:   time.Now(),
		pool:        pool,
		table:       pgx.Identifier{table},
		size:        size,
		rows:        make([][]interface{}, 0, size+1),
		columnNames: columnNames,
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
	b.rows = append(b.rows, row)
	l := len(b.rows)
	b.mu.Unlock()

	if l >= b.size {
		b.flush()
	}
}

func (b *PostgresqlBuffer) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()

	src := pgx.CopyFromRows(b.rows)
	_, err := b.pool.CopyFrom(context.Background(), b.table, b.columnNames, src)
	if err != nil {
		logger.GetLogger().Error(
			"CopyFrom failed",
			zap.String("Error", err.Error()),
		)
	}
	if src.Err() != nil {
		logger.GetLogger().Error(
			"CopyFrom failed",
			zap.String("Error", src.Err().Error()),
		)
	}
	b.rows = make([][]interface{}, 0, b.size+1)
	b.lastFlush = time.Now()
}
