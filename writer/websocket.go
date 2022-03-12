package writer

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WebsocketWriter struct {
	url        string
	self       string
	broadcast  chan message.Mapped
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	cache      Cache
}

func NewWebsocketWriter() *WebsocketWriter {
	return &WebsocketWriter{
		broadcast:  make(chan message.Mapped),
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		cache:      Cache{},
	}
}

func (ws *WebsocketWriter) WithURL(u string) *WebsocketWriter {
	ws.url = u
	return ws
}

func (ws *WebsocketWriter) WitSelf(s string) *WebsocketWriter {
	ws.self = s
	return ws
}

func (ws *WebsocketWriter) WriteMapped(subscriber mangos.Socket) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	go ws.receive(subscriber)
	go ws.run()
	http.HandleFunc("/signalk/v1/stream", ws.serveWs)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logger.GetLogger().Info(
			"Unknown request",
			zap.String("Request", r.URL.String()),
		)
	})
	err := http.ListenAndServe(ws.url, nil)
	if err != nil {
		logger.GetLogger().Fatal(
			"ListenAndServe",
			zap.String("Error", err.Error()),
		)
	}
}

func (w *WebsocketWriter) run() {
	for {
		select {
		case client := <-w.register:
			w.clients[client] = true
			message, err := json.Marshal(struct {
				Name      string    `json:"name"`
				Version   string    `json:"version"`
				Self      string    `json:"self"`
				Roles     []string  `json:"roles"`
				Timestamp time.Time `json:"timestamp"`
			}{
				Name:      "GOSK",
				Version:   "1.0.0",
				Self:      w.self,
				Roles:     []string{"main", "master"},
				Timestamp: time.Now().UTC(),
			})
			if err != nil {
				logger.GetLogger().Warn(
					"Could not marshall the hello message",
					zap.String("Error", err.Error()),
				)
			}
			client.send <- message
			for _, delta := range w.cache.retrieveAll() {
				message, err = json.Marshal(delta)
				if err != nil {
					logger.GetLogger().Warn(
						"Could not marshall the delta message",
						zap.String("Error", err.Error()),
					)
				}
				client.send <- message
			}
		case client := <-w.unregister:
			logger.GetLogger().Info(
				"Client lost",
			)
			if _, ok := w.clients[client]; ok {
				delete(w.clients, client)
				close(client.send)
			}
		case delta := <-w.broadcast:
			w.cache.injectOrUpdate(delta)
			for client := range w.clients {
				if !client.isSubscribedTo(delta) {
					continue
				}
				message, err := json.Marshal(delta)
				if err != nil {
					logger.GetLogger().Warn(
						"Could not marshall the delta message",
						zap.String("Error", err.Error()),
					)
				}
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(w.clients, client)
				}
			}
		}
	}
}

// serveWs handles websocket requests from the peer.
func (ws *WebsocketWriter) serveWs(w http.ResponseWriter, r *http.Request) {
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

	c := &Client{
		w:                ws,
		conn:             conn,
		send:             make(chan []byte, 256),
		sendCachedValues: r.URL.Query().Get("sendCachedValues") != "false",
	}

	subscribeParam := r.URL.Query().Get("subscribe")
	if subscribeParam == "none" {
		// don't subscribe to anything
	} else if subscribeParam == "all" {
		c.handleSubscribeMessages(SubscribeMessage{Context: "*", Subscribe: []SubscribeSection{{Path: "*"}}})
	} else {
		c.handleSubscribeMessages(SubscribeMessage{Context: c.w.self, Subscribe: []SubscribeSection{{Path: "*"}}})
	}

	c.w.register <- c

	go c.writePump()
	go c.readPump()
}

func (h *WebsocketWriter) receive(subscriber mangos.Socket) {
	var mapped *message.Mapped
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}

		h.broadcast <- *mapped
	}
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	w *WebsocketWriter

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	subscriptions    map[string]map[string]struct{} // map of contexts to a map of paths to empty structs
	sendCachedValues bool

	// SignalK context
	self string
}

func NewClient() Client {
	return Client{}
}

func (c *Client) handleSubscribeMessages(m SubscribeMessage) {
	if c.subscriptions == nil {
		c.subscriptions = make(map[string]map[string]struct{})
	}

	if m.Context == "vessels.self" {
		m.Context = c.self
	}

	// handle subscribe
	for _, subscription := range m.Subscribe {
		if _, ok := c.subscriptions[m.Context]; !ok {
			c.subscriptions[m.Context] = make(map[string]struct{})
		}
		c.subscriptions[m.Context][subscription.Path] = struct{}{}
	}

	// handle unsubscribe
	for _, subscription := range m.Unsubscribe {
		for context, paths := range c.subscriptions {
			if isPatternMatch([]rune(context), []rune(m.Context)) {
				for path := range paths {
					if isPatternMatch([]rune(path), []rune(subscription.Path)) {
						delete(paths, path)
					}
				}
			}
			if len(c.subscriptions[context]) == 0 {
				delete(c.subscriptions, context)
			}
		}
	}
}

func (c *Client) isSubscribedTo(m message.Mapped) bool {
	if c.subscriptions == nil {
		return false
	}
	for context, paths := range c.subscriptions {
		if paths == nil {
			continue
		}
		if isPatternMatch([]rune(m.Context), []rune(context)) {
			for path := range paths {
				for _, update := range m.Updates {
					for _, value := range update.Values {
						if isPatternMatch([]rune(value.Path), []rune(path)) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
func isPatternMatch(subject, pattern []rune) bool {
	if len(pattern) == 0 {
		return len(subject) == 0
	}

	if string(pattern) == "*" {
		return true
	}

	var deepMatch func([]rune, []rune) bool
	deepMatch = func(subject, pattern []rune) bool {
		for len(pattern) > 0 {
			if pattern[0] == '*' {
				return deepMatch(subject, pattern[1:]) || len(subject) > 0 && deepMatch(subject[1:], pattern)
			} else {
				if len(subject) == 0 || subject[0] != pattern[0] {
					return false
				}
			}
			subject = subject[1:]
			pattern = pattern[1:]
		}
		return len(subject) == 0 && len(pattern) == 0
	}

	return deepMatch(subject, pattern)
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.w.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	var m SubscribeMessage
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.GetLogger().Warn(
					"Unexpected close error",
					zap.String("Error", err.Error()),
				)
			}
			break
		}

		m = SubscribeMessage{}
		err = json.Unmarshal(message, &m)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the message",
				zap.String("Error", err.Error()),
			)
			continue
		}

		c.handleSubscribeMessages(m)
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.w.unregister <- c
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					logger.GetLogger().Warn(
						"Could not send the close message",
						zap.String("Error", err.Error()),
					)
				}
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				logger.GetLogger().Warn(
					"Could not write to client",
					zap.ByteString("Message", message),
					zap.String("Error", err.Error()),
				)
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logger.GetLogger().Warn(
					"Could not ping the client",
					zap.String("Error", err.Error()),
				)
				return
			}
		}
	}
}

type SubscribeMessage struct {
	Context     string             `json:"context"`
	Subscribe   []SubscribeSection `json:"subscribe"`
	Unsubscribe []SubscribeSection `json:"unsubscribe"`
}

type SubscribeSection struct {
	Path string `json:"path"`
}

type Cache struct {
	content map[string]map[string]message.Mapped // map of context to a map of path to a delta
}

func (c *Cache) injectOrUpdate(m message.Mapped) {
	if c.content == nil {
		c.content = make(map[string]map[string]message.Mapped)
	}
	if _, ok := c.content[m.Context]; !ok {
		c.content[m.Context] = make(map[string]message.Mapped)
	}
	for _, update := range m.Updates {
		for _, value := range update.Values {
			// make a separate message for each path
			c.content[m.Context][value.Path] = message.Mapped{
				Context: m.Context,
				Updates: []message.Update{
					{
						Source:    update.Source,
						Timestamp: update.Timestamp,
						Values: []message.Value{
							value,
						},
					},
				},
			}
		}
	}
}

func (c *Cache) retrieveAll() []message.Mapped {
	if c.content == nil {
		return []message.Mapped{}
	}

	result := make([]message.Mapped, 0)
	for _, context := range c.content {
		for _, message := range context {
			result = append(result, message)
		}
	}

	return result
}
