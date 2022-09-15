package collector

import (
	"encoding/json"
	"time"

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
	processRateLimited(stream, collector, protocol, publisher, 0)
}

func processRateLimited(stream <-chan []byte, collector string, protocol string, publisher mangos.Socket, minWait time.Duration) {
	var m *message.Raw
	lastTimestamp := time.Now()
	for value := range stream {
		logger.GetLogger().Debug(
			"Received a message from the stream",
			zap.ByteString("Message", value),
		)
		if time.Since(lastTimestamp) < minWait {
			continue
		}
		lastTimestamp = time.Now()
		m = message.NewRaw().WithCollector(collector).WithValue(value).WithType(protocol)
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
