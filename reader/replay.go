package reader

import (
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type DatabaseReader struct {
	db    *database.PostgresqlDatabase
	start time.Time
	end   time.Time
}

type RawDatabaseReader struct {
	DatabaseReader
	removeMapped bool
}

type MappedDatabaseReader struct {
	DatabaseReader
}

func NewRawDatabaseReader(c *config.DatabaseReaderConfig, removeMapped bool) *RawDatabaseReader {
	return &RawDatabaseReader{
		DatabaseReader: DatabaseReader{
			db:    database.NewPostgresqlDatabase(&c.PostgresqlConfig),
			start: c.Start,
			end:   c.End,
		},
		removeMapped: removeMapped,
	}
}

func (dbr *RawDatabaseReader) ReadRaw(publisher mangos.Socket) {
	var wg sync.WaitGroup

	rawRows, err := dbr.db.ReadRaw(` WHERE "time" BETWEEN $1 AND $2`, dbr.start, dbr.end)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve raw messages",
			zap.String("Error", err.Error()),
		)
	}

	if dbr.removeMapped {
		wg.Add(len(rawRows))
		for _, m := range rawRows {
			go func(m message.Raw) {
				err := dbr.db.DeleteMapped(m.Timestamp, m.Uuid)
				logger.GetLogger().Fatal(
					"Unable to delete mapped data",
					zap.String("Error", err.Error()),
				)
				wg.Done()
			}(m)
		}
		wg.Wait()
	}

	wg.Add(len(rawRows))
	for _, m := range rawRows {
		go func(m message.Raw) {
			nanomsg.SendRaw(&m, publisher)
			wg.Done()
		}(m)
	}
	wg.Wait()
}

func NewMappedDatabaseReader(c *config.DatabaseReaderConfig) *MappedDatabaseReader {
	return &MappedDatabaseReader{
		DatabaseReader: DatabaseReader{
			db:    database.NewPostgresqlDatabase(&c.PostgresqlConfig),
			start: c.Start,
			end:   c.End,
		},
	}
}

func (dbr *MappedDatabaseReader) ReadMapped(publisher mangos.Socket) {
	var wg sync.WaitGroup

	mappedRows, err := dbr.db.ReadMapped(` WHERE "time" BETWEEN $1 AND $2`, dbr.start, dbr.end)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve raw messages",
			zap.String("Error", err.Error()),
		)
	}

	wg.Add(len(mappedRows))
	for _, m := range mappedRows {
		go func(m message.Mapped) {
			nanomsg.SendMapped(&m, publisher)
			wg.Done()
		}(m)
	}
	wg.Wait()
}
