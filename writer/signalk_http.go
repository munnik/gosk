package writer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/lxzan/gws"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

const (
	SignalKEndpointsPath = "/signalk/"
	SignalKHTTPPath      = "/signalk/v1/api/"
	SignalKWSPath        = "/signalk/v1/stream/"
)

type server struct {
	Id      string `json:"id"`
	Version string `json:"version"`
}

type endpoint struct {
	Version     string `json:"version"`
	SignalKHTTP string `json:"signalk-http"`
	SignalKWS   string `json:"signalk-ws"`
}

type endpoints struct {
	Endpoints map[string]endpoint `json:"endpoints"`
	Server    server              `json:"server"`
}

func (w *SignalKWriter) startHTTPServer(upgrader *gws.Upgrader) error {
	mux := http.NewServeMux()
	mux.HandleFunc(SignalKWSPath, func(writer http.ResponseWriter, request *http.Request) {
		socket, err := upgrader.Upgrade(writer, request)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not upgrade to a websocket connection connection",
				zap.String("Host", w.config.URL.Host),
				zap.String("Error", err.Error()),
			)
			return
		}
		go func() {
			socket.ReadLoop()
		}()
	})
	mux.HandleFunc(SignalKHTTPPath+"{path...}", w.serveFullDataModel)
	mux.HandleFunc(SignalKEndpointsPath, w.serveEndpoints)

	logger.GetLogger().Info("SignalK server is ready to serve")
	err := http.ListenAndServe(w.config.URL.Host, mux)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not listen and serve",
			zap.String("Host", w.config.URL.Host),
			zap.String("Error", err.Error()),
		)
	}
	return err
}

func (w *SignalKWriter) serveEndpoints(rw http.ResponseWriter, r *http.Request) {
	e := endpoints{
		// TODO: detect https/wss
		Endpoints: map[string]endpoint{
			"v1": {
				Version:     w.config.Version,
				SignalKHTTP: "http://" + r.Host + SignalKHTTPPath,
				SignalKWS:   "ws://" + r.Host + SignalKWSPath,
			},
		},
		Server: server{Id: "gosk", Version: w.config.Version},
	}
	result, _ := json.Marshal(e)
	rw.Header().Set("Content-Type", "application/json")
	rw.Write(result)
}

func (w *SignalKWriter) serveFullDataModel(rw http.ResponseWriter, r *http.Request) {
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

	path := "/" + r.PathValue("path")
	jsonObj, err = jsonObj.JSONPointer(path)
	if err != nil {
		http.NotFound(rw, r)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	fmt.Fprint(rw, jsonObj.String())
}
