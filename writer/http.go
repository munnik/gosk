package writer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	endpoints = `{
  "endpoints": {
    "v3": {
      "version": "3.0.0",
      "signalk-http": "http://%s:%s/signalk/v3/api/",
      "signalk-ws": "ws://%s:%s/signalk/v3/stream"
    }
  },
  "server": {
    "id": "gosk",
    "version": "%s"
  }
}`
)

type HTTPWriter struct {
	config *config.SignalKConfig
	db     *database.PostgresqlDatabase
	bc     *database.BigCache
	wg     *sync.WaitGroup
}

func NewHTTPWriter(c *config.SignalKConfig) *HTTPWriter {
	return &HTTPWriter{
		config: c,
		db:     database.NewPostgresqlDatabase(c.PostgresqlConfig),
		bc:     database.NewBigCache(c.BigCacheConfig),
		wg:     &sync.WaitGroup{},
	}
}

func (w *HTTPWriter) WriteMapped(subscriber mangos.Socket) {
	// fill the cache with data from the database
	w.wg.Add(1)
	go w.readFromDatabase()
	go w.update(subscriber)

	// handle route using handler function
	http.HandleFunc("/signalk", w.serveEndpoints)
	http.HandleFunc("/signalk/v3/api/", w.serverV3API)

	// listen to port
	err := http.ListenAndServe(fmt.Sprintf("%s", w.config.URL.Host), nil)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not listen and serve",
			zap.String("Host", w.config.URL.Host),
			zap.String("Error", err.Error()),
		)
	}
}

func (w *HTTPWriter) serveEndpoints(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(rw, endpoints, w.config.URL.Hostname(), w.config.URL.Port(), w.config.URL.Hostname(), w.config.URL.Port(), w.config.Version)
}

func (w *HTTPWriter) serverV3API(rw http.ResponseWriter, r *http.Request) {
	w.wg.Wait()

	mapped, err := w.bc.ReadMapped("")
	if err != nil {
		rw.WriteHeader(400)
		fmt.Printf("Error occurred while retrieving data, please see the server logs for more details")
		return
	}

	jsonObj := gabs.New()
	jsonObj.Set("1.5.0", "version")
	jsonObj.Set(w.config.SelfContext, "self")

	var jsonPath []string
	for _, m := range mapped {
		for _, u := range m.Updates {
			for _, v := range u.Values {
				jsonPath = strings.SplitN(m.Context, ".", 1)
				jsonPath = append(jsonPath, strings.Split(v.Path, ".")...)

				jsonObj.Set(v.Value, append(jsonPath, "value")...)
				jsonObj.Set(u.Timestamp, append(jsonPath, "timestamp")...)
				jsonObj.Set(u.Source.Label, append(jsonPath, "source", "label")...)
				jsonObj.Set(u.Source.Type, append(jsonPath, "source", "type")...)
			}
		}
	}

	rw.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(rw, jsonObj.String())
}

func (w *HTTPWriter) readFromDatabase() {
	appendToQuery := `
		INNER JOIN 
			(
				SELECT  
					"context" AS "max_context", 
					"path" AS "max_path",
					MAX("time") AS "max_time"
				FROM 
					"mapped_data" 
				GROUP BY 
					1, 2
			) "max" 
		ON 
			"time" = "max_time" AND 
			"context" = "max_context" AND 
			"path" = "max_path"
		WHERE
			"time" > $1
		;
	`
	mapped, err := w.db.ReadMapped(appendToQuery, time.Now().Add(-time.Second*time.Duration(w.config.BigCacheConfig.LifeWindow)))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve all mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	w.bc.WriteMapped(mapped...)
	w.wg.Done()
}

func (w *HTTPWriter) update(subscriber mangos.Socket) {
	var mapped message.Mapped
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, &mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		w.bc.WriteMapped(mapped)
	}
}
