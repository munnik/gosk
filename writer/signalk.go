package writer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	SignalKEndpointsPath = "/signalk"
	SignalKHTTPPath      = "/signalk/v1/api/"
	SignalKWSPath        = "/signalk/v1/stream"
)

type SignalKWriter struct {
	config           *config.SignalKConfig
	database         *database.PostgresqlDatabase
	cache            *database.BigCache
	wg               *sync.WaitGroup
	mu               sync.Mutex
	websocketClients map[string]websocketClient
}

func NewSignalKWriter(c *config.SignalKConfig) *SignalKWriter {
	return &SignalKWriter{
		config:           c,
		database:         database.NewPostgresqlDatabase(c.PostgresqlConfig),
		cache:            database.NewBigCache(c.BigCacheConfig),
		wg:               &sync.WaitGroup{},
		websocketClients: make(map[string]websocketClient, 0),
	}
}

func (w *SignalKWriter) WriteMapped(subscriber mangos.Socket) {
	// fill the cache with data from the database
	w.wg.Add(1)
	go w.readFromDatabase()
	go w.receive(subscriber)

	router := chi.NewRouter()
	router.Use(middleware.Compress(5))

	router.Get(SignalKHTTPPath+"*", w.serveFullDataModel)
	router.Get(SignalKEndpointsPath, w.serveEndpoints)
	router.Get(SignalKWSPath, w.serveWebsocket)

	// listen to port
	err := http.ListenAndServe(fmt.Sprintf("%s", w.config.URL.Host), router)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not listen and serve",
			zap.String("Host", w.config.URL.Host),
			zap.String("Error", err.Error()),
		)
	}
}

func (w *SignalKWriter) receive(subscriber mangos.Socket) {
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		var mapped message.Mapped
		if err := json.Unmarshal(received, &mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		go w.updateFullDataModel(mapped)
		go w.updateWebsocket(mapped)
	}
}

func (s *SignalKWriter) addClient(c websocketClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.websocketClients[c.host] = c
}

func (s *SignalKWriter) removeClient(c websocketClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.websocketClients, c.host)
	// close(c.deltas)
}

func (s *SignalKWriter) getClients() []websocketClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]websocketClient, 0, len(s.websocketClients))
	for _, c := range s.websocketClients {
		result = append(result, c)
	}
	return result
}
