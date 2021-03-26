// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signalkws

import (
	"time"

	"github.com/goccy/go-json"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/nanomsg"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan deltaMessage

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	cache cacheType
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan deltaMessage),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		cache:      cacheType{},
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			message, err := json.Marshal(helloMessage{
				Name:      "GOSK",
				Version:   "1.0.0",
				Self:      self,
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
			for _, delta := range h.cache.retrieveAll() {
				message, err = json.Marshal(delta)
				if err != nil {
					logger.GetLogger().Warn(
						"Could not marshall the delta message",
						zap.String("Error", err.Error()),
					)
				}
				client.send <- message
			}
		case client := <-h.unregister:
			logger.GetLogger().Info(
				"Client lost",
			)
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case delta := <-h.broadcast:
			h.cache.injectOrUpdate(delta)
			for client := range h.clients {
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
					delete(h.clients, client)
				}
			}
		}
	}
}

func (h *Hub) receive(socket mangos.Socket) {
	m := &nanomsg.MappedData{}
	for {
		received, err := socket.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Error on receiving data from the socket",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := proto.Unmarshal(received, m); err != nil {
			logger.GetLogger().Warn(
				"Error on unmarshalling the received data",
				zap.String("Error", err.Error()),
			)
			continue
		}

		h.broadcast <- deltaMessage{
			Context: m.Context,
			Updates: []updateSection{
				{
					Source: sourceSection{
						Label: m.Header.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE],
					},
					Timestamp: time.Now().UTC(),
					Values: []valueSection{
						{
							Path:  m.Path,
							Value: m.Value,
						},
					},
				},
			},
		}
	}
}
