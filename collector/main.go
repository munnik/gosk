package collector

import (
	"encoding/json"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// Collector interface
type Collector interface {
	Collect(publisher mangos.Socket)
}

func process(stream <-chan []byte, collector string, protocol string, publisher mangos.Socket) {
	var m *message.Raw
	for value := range stream {
		logger.GetLogger().Debug(
			"Received a message from the stream",
			zap.ByteString("Message", value),
		)

		m = message.NewRaw().WithCollector(collector).WithValue(value).WithType(protocol)
		toSend, err := json.Marshal(m)
		if err != nil {
			logger.GetLogger().Warn(
				"Unable to marshall the message to JSON",
				zap.ByteString("Message", value),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := publisher.Send(toSend); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", value),
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
