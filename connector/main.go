package connector

import (
	"os"
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

const bufferCapacity = 5000

// Connector interface
type Connector[T nanomsg.Message] interface {
	Publish(publisher *nanomsg.Publisher[T])
	Subscribe(subscriber *nanomsg.Subscriber[T])
}

func process(stream <-chan []byte, connector string, protocol string, publisher *nanomsg.Publisher[message.Raw], timeout *time.Timer, timeoutDuration time.Duration) {
	sendBuffer := make(chan *message.Raw, bufferCapacity)
	defer close(sendBuffer)
	go publisher.Send(sendBuffer)

	var m *message.Raw
	for value := range stream {
		timeout.Reset(timeoutDuration)
		m = message.NewRaw().WithConnector(connector).WithValue(value).WithType(protocol)
		sendBuffer <- m
	}
}

func exit() {
	logger.GetLogger().Warn("timeout receiving data for the stream, no data received")
	os.Exit(0)

}
