package connector

import (
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

const bufferCapacity = 5000

// Connector interface
type Connector[T nanomsg.Message] interface {
	Publish(publisher *nanomsg.Publisher[T])
	Subscribe(subscriber *nanomsg.Subscriber[T])
}

func process(stream <-chan []byte, connector string, protocol string, publisher *nanomsg.Publisher[message.Raw]) {
	sendBuffer := make(chan *message.Raw, bufferCapacity)
	go publisher.Send(sendBuffer)

	var m *message.Raw
	for value := range stream {
		m = message.NewRaw().WithConnector(connector).WithValue(value).WithType(protocol)
		sendBuffer <- m
	}
}
