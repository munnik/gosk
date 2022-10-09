package connector

import (
	"encoding/json"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// Connector interface
type Connector interface {
	Connect(publisher mangos.Socket)
}

func process(stream <-chan []byte, connector string, protocol string, publisher mangos.Socket) {
	var m *message.Raw
	for value := range stream {
		logger.GetLogger().Debug(
			"Received a message from the stream",
			zap.ByteString("Message", value),
		)

		m = message.NewRaw().WithConnector(connector).WithValue(value).WithType(protocol)
		bytes, err := json.Marshal(m)
		if err != nil {
			logger.GetLogger().Warn(
				"Unable to marshall the message to JSON",
				zap.ByteString("Message", value),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := publisher.Send(bytes); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
		logger.GetLogger().Debug(
			"Send the message on the NanoMSG socket",
			zap.ByteString("Message", value),
		)
	}
}
