package reader

import (
	"encoding/json"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type PostgresqlReader struct {
	db  *database.PostgresqlDatabase
	wcs []database.WhereClause
}

func NewPostgresqlReader(c *config.PostgresqlConfig) *PostgresqlReader {
	return &PostgresqlReader{db: database.NewPostgresqlDatabase(c)}
}

func (r *PostgresqlReader) ReadMapped(publisher mangos.Socket) {
	for _, wc := range r.wcs {
		mappedRows, err := r.db.ReadMapped(wc)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not read mapped data from the database",
				zap.String("WhereClause", wc.String()),
			)
			continue
		}
		for _, m := range mappedRows {
			bytes, err := json.Marshal(m)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not marshal delta",
					zap.String("Error", err.Error()),
				)
				continue
			}
			publisher.Send(bytes)
		}
	}
}
