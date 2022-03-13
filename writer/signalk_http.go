package writer

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

//go:embed templates/*
var fs embed.FS

func (w *SignalKWriter) serveEndpoints(rw http.ResponseWriter, r *http.Request) {
	t, err := template.ParseFS(fs, "templates/endpoints.json")
	if err != nil {
		http.NotFound(rw, r)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	data := struct {
		Host     string
		Version  string
		HTTPPath string
		WSPath   string
	}{
		Host:     r.Host,
		Version:  w.config.Version,
		HTTPPath: SignalKHTTPPath,
		WSPath:   SignalKWSPath,
	}
	t.Execute(rw, data)
}

func (w *SignalKWriter) serveFullDataModel(rw http.ResponseWriter, r *http.Request) {
	w.wg.Wait()

	mapped, err := w.bc.ReadMapped("")
	if err != nil {
		http.NotFound(rw, r)
		return
	}

	jsonObj := gabs.New()
	jsonObj.Set("1.5.0", "version")
	jsonObj.Set(w.config.SelfContext, "self")

	var jsonPath []string
	for _, m := range mapped {
		for _, u := range m.Updates {
			for _, v := range u.Values {
				jsonPath = strings.SplitN(m.Context, ".", 2)
				jsonPath = append(jsonPath, strings.Split(v.Path, ".")...)

				jsonObj.Set(v.Value, append(jsonPath, "value")...)
				jsonObj.Set(u.Timestamp, append(jsonPath, "timestamp")...)
				jsonObj.Set(u.Source.Label, append(jsonPath, "source", "label")...)
				jsonObj.Set(u.Source.Type, append(jsonPath, "source", "type")...)
			}
		}
	}

	searchPath := strings.Replace(r.URL.String(), SignalKHTTPPath, "", 1)
	if searchPath == "" {
		rw.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(rw, jsonObj.String())
		return
	}

	searchPath = "/" + searchPath
	jsonObj, err = jsonObj.JSONPointer(searchPath)
	if err != nil {
		http.NotFound(rw, r)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(rw, jsonObj.String())
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

func (w *SignalKWriter) updateFullDataModel(mapped message.Mapped) {
	w.bc.WriteMapped(mapped)
}
