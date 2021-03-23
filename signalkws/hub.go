// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signalkws

import (
	"fmt"
	"time"

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
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			client.send <- []byte(fmt.Sprintf(helloTemplate, time.Now().UTC().Format(time.RFC3339)))
		case client := <-h.unregister:
			logger.GetLogger().Info(
				"Client lost",
			)
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
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
	var valueAsString string
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

		switch m.Datatype {
		case nanomsg.DOUBLE:
			valueAsString = fmt.Sprintf("%f", m.DoubleValue)
		case nanomsg.STRING:
			valueAsString = fmt.Sprintf(`"%s"`, m.StringValue)
		case nanomsg.POSITION:
			valueAsString = fmt.Sprintf(`{"latitude": %f, "longitude": %f, "altitude": %f}`, m.PositionValue.Latitude, m.PositionValue.Longitude, m.PositionValue.Altitude)
		case nanomsg.LENGTH:
			valueAsString = fmt.Sprintf(`{"overall": %f, "hull": %f, "waterline": %f}`, m.LengthValue.Overall, m.LengthValue.Hull, m.LengthValue.Waterline)
		default:
			continue
		}
		h.broadcast <- []byte(
			fmt.Sprintf(
				deltaTemplate,
				m.Context,
				m.Header.HeaderSegments[nanomsg.HEADERSEGMENTSOURCE],
				m.Timestamp.AsTime().UTC().Format(time.RFC3339),
				m.Path,
				valueAsString,
			),
		)
	}
}
