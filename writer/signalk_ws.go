package writer

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lxzan/gws"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

const (
	PingInterval = 5 * time.Second
	PingWait     = 10 * time.Second
)

type Handler struct {
	w        *SignalKWriter
	sessions *gws.ConcurrentMap[string, *gws.Conn]
}

func NewHandler(w *SignalKWriter) *Handler {
	result := &Handler{w: w, sessions: gws.NewConcurrentMap[string, *gws.Conn](16)}
	go result.ping()

	return result
}

func (h *Handler) getName(socket *gws.Conn) string {
	name, exist := socket.Session().Load("name")
	if exist {
		return name.(string)
	} else {
		name = uuid.Must(uuid.NewUUID()).String()
		socket.Session().Store("name", name)
	}

	return name.(string)
}

func (h *Handler) OnOpen(socket *gws.Conn) {
	name := h.getName(socket)
	if conn, ok := h.sessions.Load(name); ok {
		conn.WriteClose(1000, []byte("connection replaced"))
	}
	_ = socket.SetDeadline(time.Now().Add(PingInterval + PingWait))
	h.sessions.Store(name, socket)
	logger.GetLogger().Info(
		"New websocket connection",
		zap.String("Name", name),
	)
	socket.WriteMessage(
		gws.OpcodeText,
		[]byte(fmt.Sprintf(`{"name":"gosk","version":"%s","timestamp":"%s","self":"%s","roles": ["master", "main"]}`, h.w.config.Version, time.Now().UTC().Format(time.RFC3339), h.w.config.SelfContext)),
	)
}

func (h *Handler) OnClose(socket *gws.Conn, err error) {
	name := h.getName(socket)
	if _, ok := h.sessions.Load(name); ok {
		h.sessions.Delete(name)
	}
	logger.GetLogger().Warn(
		"Websocket connection closed",
		zap.String("Name", name),
		zap.Error(err),
	)
}

func (h *Handler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(PingInterval + PingWait))
	_ = socket.WritePong(nil)
}

func (h *Handler) OnPong(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(PingInterval + PingWait))
}

// handles incomming messages from the client
func (h *Handler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	name, _ := socket.Session().Load("name")
	logger.GetLogger().Info(
		"Received a message from the client",
		zap.Any("Name", name),
		zap.ByteString("Message", message.Bytes()),
	)
}

func (h *Handler) Broadcast(message *message.Mapped) {
	payload, err := json.Marshal(message)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshal the message",
			zap.Error(err),
		)
	}
	b := gws.NewBroadcaster(gws.OpcodeText, payload)
	defer b.Close()
	h.sessions.Range(func(key string, conn *gws.Conn) bool {
		b.Broadcast(conn)
		return true
	})
}

func (h *Handler) ping() {
	ticker := time.NewTicker(PingInterval)

	for range ticker.C {
		h.sessions.Range(func(key string, conn *gws.Conn) bool {
			conn.WritePing([]byte("Ping"))
			return true
		})
	}
}
