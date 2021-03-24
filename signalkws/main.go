package signalkws

import (
	"net/http"
	"time"

	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const self = "vessels.urn:mrn:imo:mmsi:244770688"

type hello struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Self      string    `json:"self"`
	Roles     []string  `json:"roles"`
	Timestamp time.Time `json:"timestamp"`
}

type delta struct {
	Context string   `json:"context"`
	Updates []update `json:"updates"`
}

type update struct {
	Source    source    `json:"source"`
	Timestamp time.Time `json:"timestmap"`
	Values    []value   `json:"values"`
}

type source struct {
	Label string `json:"label"`
}

type value struct {
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

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

	subscribeContext := self
	if q := r.URL.Query().Get("subscribe"); len(q) > 0 {
		if q == "all" {
			subscribeContext = "*"
		} else if q == "none" {
			subscribeContext = ""
		}
	}
	client := &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
		subscription: []subscription{
			{context: subscribeContext, paths: []string{"*"}},
		},
	}
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
