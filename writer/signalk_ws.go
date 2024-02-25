package writer

import (
	"context"
	"net/http"
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type hello struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Self      string    `json:"self"`
	Roles     []string  `json:"roles"`
}

type websocketClient struct {
	host   string
	deltas chan *message.Mapped
}

func (w *SignalKWriter) serveWebsocket(rw http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(rw, r, nil)
	if err != nil {
		logger.GetLogger().Warn(
			"Unable to accept a websocket connection",
			zap.String("Error", err.Error()),
			zap.String("Request", r.RequestURI),
		)
		return
	}
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	ctx, cancel := context.WithTimeout(r.Context(), time.Second*60)
	defer cancel()

	err = wsjson.Write(ctx, c, w.createHello())
	if err != nil {
		logger.GetLogger().Warn(
			"Error while writing hello message",
			zap.String("Error", err.Error()),
		)
		return
	}

	client := websocketClient{
		deltas: make(chan *message.Mapped),
		host:   r.RemoteAddr,
	}
	w.addClient(client)
	defer w.removeClient(client)

	for {
		err = wsjson.Write(ctx, c, <-client.deltas)
		if err != nil {
			logger.GetLogger().Warn(
				"Error while writing delta message to the client, closing the connection",
				zap.String("Error", err.Error()),
				zap.String("Client address", r.RemoteAddr),
			)
			c.Close(websocket.StatusGoingAway, "error while writing")
			return
		}
	}
}

func (w *SignalKWriter) updateWebsocket(message *message.Mapped) {
	for _, c := range w.getClients() {
		c.deltas <- message
	}
}

func (w *SignalKWriter) createHello() hello {
	return hello{
		Name:      "gosk",
		Version:   w.config.Version,
		Timestamp: time.Now(),
		Self:      w.config.SelfContext,
		Roles:     []string{"master", "main"},
	}
}
