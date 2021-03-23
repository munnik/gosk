package signalkws

import (
	"net/http"

	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const helloTemplate = `{"name":"GOSK","version":"1.0.0","self":"vessels.urn:mrn:imo:mmsi:244770688","roles":["master","main"],"timestamp":"%s"}`
const deltaTemplate = `{"context":"%s","updates":[{"source":{"label":"%s"},"timestamp":"%s","values":[{"path":"%s","value":%s}]}]}`

// serveWs handles websocket requests from the peer.
func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	logger.GetLogger().Info(
		"Client connected",
		zap.String("Request", r.URL.String()),
	)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.GetLogger().Warn(
			"Unable to upgrade the http connection to a websocket",
			zap.String("Error", err.Error()),
		)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	go client.writePump()
}

// StoreKeyValue saves all received key value messages in the database
func Start(socket mangos.Socket) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	hub := newHub()
	go hub.receive(socket)
	go hub.run()
	http.HandleFunc("/signalk/v1/stream", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.GetLogger().Info(
			"Unknown request",
			zap.String("Request", r.URL.String()),
		)
	})
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		logger.GetLogger().Fatal(
			"ListenAndServe",
			zap.String("Error", err.Error()),
		)
	}
}
