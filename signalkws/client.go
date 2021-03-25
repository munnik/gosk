// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signalkws

import (
	"time"

	"github.com/goccy/go-json"

	"github.com/gorilla/websocket"
	"github.com/munnik/gosk/logger"
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

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	subscriptions    map[string]map[string]struct{} // map of context to a map of paths to empty structs
	sendCachedValues bool
}

func (c *Client) handleSubscribeMessages(m subscribeMessage) {
	if c.subscriptions == nil {
		c.subscriptions = make(map[string]map[string]struct{})
	}

	if m.Context == "vessels.self" {
		m.Context = self
	}

	// handle subscriptions
	for _, subscription := range m.Subscribe {
		if _, ok := c.subscriptions[m.Context]; !ok {
			c.subscriptions[m.Context] = make(map[string]struct{})
		}
		c.subscriptions[m.Context][subscription.Path] = struct{}{}
	}

	// handle unsubscriptions
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

func (c *Client) isSubscribedTo(m deltaMessage) bool {
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
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	var m subscribeMessage
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

		m = subscribeMessage{}
		err = json.Unmarshal(message, &m)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshall the message",
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
		c.hub.unregister <- c
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
