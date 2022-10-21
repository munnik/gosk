package collector

import (
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
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

		nanomsg.SendRaw(m, publisher)
	}
}
