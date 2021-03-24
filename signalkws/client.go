// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signalkws

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = 60 * time.Second
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type subscription struct {
	context string
	paths   []string
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	subscription     []subscription
	sendCachedValues bool
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
func (c *Client) isSubscribedTo(d delta) bool {
	for _, subscription := range c.subscription {
		if subscription.context == "*" || subscription.context == d.Context {
			for _, subscribedPath := range subscription.paths {
				if subscribedPath == "*" {
					return true
				}
				for _, update := range d.Updates {
					for _, value := range update.Values {
						if subscribedPath == value.Path {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
