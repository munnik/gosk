package writer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

type endpoint struct {
	Version     string `json:"version"`
	SignalKHTTP string `json:"signalk-http"`
	SignalKWS   string `json:"signalk-ws"`
}
type server struct {
	Id      string `json:"id"`
	Version string `json:"version"`
}
type endpoints struct {
	Endpoints map[string]endpoint `json:"endpoints"`
	Server    server              `json:"server"`
}

func (w *SignalKWriter) serveEndpoints(rw http.ResponseWriter, r *http.Request) {
	e := endpoints{
		// TODO: detect https/wss
		Endpoints: map[string]endpoint{"v1": {Version: w.config.Version, SignalKHTTP: "http://" + r.Host + SignalKHTTPPath, SignalKWS: "ws://" + r.Host + SignalKWSPath}},
		Server:    server{Id: "gosk", Version: w.config.Version},
	}
	result, _ := json.Marshal(e)
	rw.Header().Set("Content-Type", "application/json")
	rw.Write(result)
}

func (w *SignalKWriter) serveFullDataModel(rw http.ResponseWriter, r *http.Request) {
	w.wg.Wait()

	mapped, err := w.cache.ReadMapped("")
	if err != nil {
		http.NotFound(rw, r)
		return
	}

	jsonObj := gabs.New()
	jsonObj.Set(w.config.Version, "version")
	jsonObj.Set(w.config.SelfContext, "self")

	var jsonPath []string
	for _, m := range mapped {
		for _, sm := range m.ToSingleValueMapped() {
			jsonPath = strings.SplitN(sm.Context, ".", 2)

			if sm.Path == "" {
				// if path is empty don't include source and timestamp
				if vesselInfo, ok := sm.Value.(message.VesselInfo); ok {
					if vesselInfo.MMSI != nil {
						jsonObj.Set(vesselInfo.MMSI, append(jsonPath, "mmsi")...)
					}
					if vesselInfo.Name != nil {
						jsonObj.Set(vesselInfo.Name, append(jsonPath, "name")...)
					}
				}
				continue
			}

			jsonPath = append(jsonPath, strings.Split(sm.Path, ".")...)

			jsonObj.Set(sm.Value, append(jsonPath, "value")...)
			jsonObj.Set(sm.Timestamp, append(jsonPath, "timestamp")...)
			jsonObj.Set(sm.Source.Label, append(jsonPath, "source", "label")...)
			jsonObj.Set(sm.Source.Type, append(jsonPath, "source", "type")...)
			jsonObj.Set(sm.Source.Uuid, append(jsonPath, "source", "uuid")...)
		}
	}

	searchPath := strings.Replace(r.URL.String(), SignalKHTTPPath, "", 1)
	if searchPath == "" {
		rw.Header().Set("Content-Type", "application/json")
		fmt.Fprint(rw, jsonObj.String())
		return
	}

	searchPath = "/" + searchPath
	jsonObj, err = jsonObj.JSONPointer(searchPath)
	if err != nil {
		http.NotFound(rw, r)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	fmt.Fprint(rw, jsonObj.String())
}

func (w *SignalKWriter) readFromDatabase() {
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
	mapped, err := w.database.ReadMapped(appendToQuery, time.Now().Add(-time.Second*time.Duration(w.config.BigCacheConfig.LifeWindow)))
	if err != nil {
		logger.GetLogger().Warn(
			"Could not retrieve all mapped data from database",
			zap.String("Error", err.Error()),
		)
		return
	}
	w.cache.WriteMapped(mapped...)
	w.wg.Done()
}

func (w *SignalKWriter) updateFullDataModel(mapped *message.Mapped) {
	w.cache.WriteMapped(mapped)
}
