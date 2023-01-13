package writer

import (
	"encoding/json"
	"sync"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	bufferSize  = 100
	noOfWorkers = 10
)

type PostgresqlWriter struct {
	db            *database.PostgresqlDatabase
	mappedChannel chan message.Mapped
	rawChannel    chan message.Raw
}

func NewPostgresqlWriter(c *config.PostgresqlConfig) *PostgresqlWriter {
	mappedChannel := make(chan message.Mapped, bufferSize)
	rawChannel := make(chan message.Raw, bufferSize)
	return &PostgresqlWriter{db: database.NewPostgresqlDatabase(c), mappedChannel: mappedChannel, rawChannel: rawChannel}
}

func (w *PostgresqlWriter) StartRawWorkers() {
	var wg sync.WaitGroup
	wg.Add(noOfWorkers)
	for i := 0; i < noOfWorkers; i++ {
		go func() {
			for raw := range w.rawChannel {
				w.db.WriteRaw(raw)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func (w *PostgresqlWriter) WriteRaw(subscriber mangos.Socket) {
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		raw := message.Raw{}
		if err := json.Unmarshal(received, &raw); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.rawChannel <- raw
	}
}

func (w *PostgresqlWriter) StartMappedWorkers() {
	var wg sync.WaitGroup
	wg.Add(noOfWorkers)
	for i := 0; i < noOfWorkers; i++ {
		go func() {
			for mapped := range w.mappedChannel {
				w.db.WriteMapped(mapped)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

func (w *PostgresqlWriter) WriteMapped(subscriber mangos.Socket) {
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		mapped := message.Mapped{}
		if err := json.Unmarshal(received, &mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.mappedChannel <- mapped
	}
}
