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
	go result.startTicker(flushInterval)

	return result
}

func (b *PostgresqlBuffer) Add(row []interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rows = append(b.rows, row)

	if len(b.rows) >= b.size {
		b.flush()
	}
}

func (b *PostgresqlBuffer) startTicker(flushInterval int) {
	ticker := time.NewTicker(time.Second * time.Duration(flushInterval))
	for {
		<-ticker.C
		b.mu.Lock()
		if time.Now().Add(-time.Second * time.Duration(flushInterval)).Before(b.lastFlush) {
			b.flush()
		}
		defer b.mu.Unlock()
	}
}

func (b *PostgresqlBuffer) flush() {
	c := make([][]interface{}, len(b.rows))
	copy(c, b.rows)
	go b.copyFrom(pgx.CopyFromRows(c))
	b.rows = make([][]interface{}, 0, b.size+1)
	b.lastFlush = time.Now()

}

func (b *PostgresqlBuffer) copyFrom(src pgx.CopyFromSource) {
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
}
