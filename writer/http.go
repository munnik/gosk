package writer

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/Jeffail/gabs/v2"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	endpoints = `{
  "endpoints": {
    "v3": {
      "version": "3.0.0",
      "signalk-http": "http://%s:%s/signalk/v3/api/",
      "signalk-ws": "ws://%s:%s/signalk/v3/stream",
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
	}
}

func (a *HTTPWriter) WriteMapped(subscriber mangos.Socket) {
	// fill the cache with data from the database
	a.wg.Add(1)
	go a.readFromDatabase()

	// handle route using handler function
	http.HandleFunc("/signalk", a.serveEndpoints)
	http.HandleFunc("/signalk/v3/api/", a.serverV3API)

	// listen to port
	err := http.ListenAndServe(fmt.Sprintf("%s", a.config.URL.Host), nil)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not listen and serve",
			zap.String("Host", a.config.URL.Host),
			zap.String("Error", err.Error()),
		)
	}
}

func (a *HTTPWriter) serveEndpoints(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, endpoints, a.config.URL.Hostname(), a.config.URL.Port(), a.config.URL.Hostname(), a.config.URL.Port(), a.config.Version)
}

func (a *HTTPWriter) serverV3API(w http.ResponseWriter, r *http.Request) {
	a.wg.Wait()

	mapped, err := a.bc.ReadMapped("")
	if err != nil {
		w.WriteHeader(400)
		fmt.Printf("Error occurred while retrieving data, please see the server logs for more details")
		return
	}

	jsonObj := gabs.New()
	jsonObj.Set("1.5.0", "version")
	jsonObj.Set(a.config.SelfContext, "self")

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

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, jsonObj.String())
}

func (a *HTTPWriter) readFromDatabase() {
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
		;
	`
	mapped, err := a.db.ReadMapped(appendToQuery)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve all mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	a.bc.WriteMapped(mapped...)
	a.wg.Done()
}
