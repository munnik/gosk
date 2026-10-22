package writer

import (
	"encoding/json"
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
		jsonData, _ := json.MarshalIndent(*mapped, "", "  ")
		fmt.Println(string(jsonData))
	}
}

func (w *StdOutWriter) WriteRaw(subscriber *nanomsg.Subscriber[message.Raw]) {
	type rawBytes struct {
		Connector string
		Timestamp time.Time
		Type      string
		Uuid      uuid.UUID
		Value     string
	}
	var rb = rawBytes{}
	receiveBuffer := make(chan *message.Raw, bufferCapacity)
	defer close(receiveBuffer)
	go subscriber.Receive(receiveBuffer)

	for raw := range receiveBuffer {
		rb.Connector = raw.Connector
		rb.Timestamp = raw.Timestamp
		rb.Type = raw.Type
		rb.Uuid = raw.Uuid
		rb.Value = fmt.Sprintf("%v", raw.Value)
		jsonData, _ := json.MarshalIndent(rb, "", "  ")
		fmt.Println(string(jsonData))
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
		jsonData, _ := json.MarshalIndent(rs, "", "  ")
		fmt.Println(string(jsonData))
	}
}
