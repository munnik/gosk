package writer

import (
	"net/http"
	"time"

	"github.com/lxzan/gws"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

const (
	SignalKEndpointsPath = "/signalk"
	SignalKHTTPPath      = "/signalk/v1/api/"
	SignalKWSPath        = "/signalk/v1/stream"
)

type SignalKWriter struct {
	config   *config.SignalKConfig
	database *database.PostgresqlDatabase
	cache    *database.BigCache
}

func NewSignalKWriter(c *config.SignalKConfig) *SignalKWriter {
	return &SignalKWriter{
		config:   c,
		database: database.NewPostgresqlDatabase(c.PostgresqlConfig),
		cache:    database.NewBigCache(c.BigCacheConfig),
	}
}

func (w *SignalKWriter) WriteMapped(subscriber *nanomsg.Subscriber[message.Mapped]) {
	// fill the cache with data from the database
	w.readFromDatabase()
	h := NewHanlder(w)
	upgrader := gws.NewUpgrader(h, &gws.ServerOption{
		ParallelEnabled:   true,                                 // Parallel message processing
		Recovery:          gws.Recovery,                         // Exception recovery
		PermessageDeflate: gws.PermessageDeflate{Enabled: true}, // Enable compression
	})
	logger.GetLogger().Info("SignalK server is ready to serve")

	go func() {
		receiveBuffer := make(chan *message.Mapped, bufferCapacity)
		defer close(receiveBuffer)
		go subscriber.Receive(receiveBuffer)
		for mapped := range receiveBuffer {
			w.updateFullDataModel(mapped)
			h.Broadcast(mapped)
		}
	}()

	http.HandleFunc(SignalKWSPath, func(writer http.ResponseWriter, request *http.Request) {
		socket, err := upgrader.Upgrade(writer, request)
		if err != nil {
			return
		}
		go func() {
			socket.ReadLoop() // Blocking prevents the context from being GC.
		}()
	})
	http.HandleFunc(SignalKEndpointsPath, w.serveEndpoints)
	http.HandleFunc(SignalKHTTPPath+"*", w.serveFullDataModel)

	// listen to port
	err := http.ListenAndServe(w.config.URL.Host, nil)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not listen and serve",
			zap.String("Host", w.config.URL.Host),
			zap.String("Error", err.Error()),
		)
	}
}

func (w *SignalKWriter) readFromDatabase() {
	mapped, err := w.database.ReadMostRecentMapped(time.Now().Add(-time.Second * time.Duration(w.config.BigCacheConfig.LifeWindow)))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve the most recent mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	w.cache.WriteMapped(mapped...)
}

func (w *SignalKWriter) updateFullDataModel(mapped *message.Mapped) {
	w.cache.WriteMapped(mapped)
}
