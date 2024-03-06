package writer

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type StdOutWriter struct {
}

func NewStdOutWriter() *StdOutWriter {
	return &StdOutWriter{}
}

func (w *StdOutWriter) WriteMapped(subscriber *nanomsg.Subscriber[message.Mapped]) {
	receiveBuffer := make(chan *message.Mapped, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	for mapped := range receiveBuffer {
		fmt.Println(mapped)
		subscriber.ReturnToPool(mapped)
	}
}

func (w *StdOutWriter) WriteRaw(subscriber *nanomsg.Subscriber[message.Raw]) {
	receiveBuffer := make(chan *message.Raw, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	for raw := range receiveBuffer {
		fmt.Println(raw)
		subscriber.ReturnToPool(raw)
	}
}

func (w *StdOutWriter) WriteRawString(subscriber *nanomsg.Subscriber[message.Raw]) {
	type rawString struct {
		Connector string
		Timestamp time.Time
		Type      string
		Uuid      uuid.UUID
		Value     string
	}
	var rs = rawString{}
	receiveBuffer := make(chan *message.Raw, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	for raw := range receiveBuffer {
		rs.Connector = raw.Connector
		rs.Timestamp = raw.Timestamp
		rs.Type = raw.Type
		rs.Uuid = raw.Uuid
		rs.Value = string(raw.Value)
		fmt.Println(rs)
		subscriber.ReturnToPool(raw)
	}
}
