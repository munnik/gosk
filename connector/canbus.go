package connector

import (
	"encoding/binary"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"

	"github.com/brutella/can"
)

type CanBusConnector struct {
	config  *config.ConnectorConfig
	timeout *time.Timer
}

func NewCanBusConnector(c *config.ConnectorConfig) (*CanBusConnector, error) {
	return &CanBusConnector{config: c,
		timeout: time.AfterFunc(c.Timeout, exit)}, nil
}

func (r *CanBusConnector) Publish(publisher *nanomsg.Publisher[message.Raw]) {
	stream := make(chan []byte, 1)
	defer close(stream)
	go func() {
		for {
			if err := r.receive(stream); err != nil {
				logger.GetLogger().Warn(
					"Error while receiving data for the stream",
					zap.String("URL", r.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
	process(stream, r.config.Name, r.config.Protocol, publisher, r.timeout, r.config.Timeout)
}

func (*CanBusConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	// do nothing
}

func (r *CanBusConnector) receive(stream chan<- []byte) error {
	bus, err := can.NewBusForInterfaceWithName(r.config.URL.Host)
	if err != nil {
		return err
	}
	defer bus.Disconnect()
	bus.SubscribeFunc(r.handleCanFrameStream(stream))
	bus.ConnectAndPublish()
	return nil
}

func (r *CanBusConnector) handleCanFrameStream(stream chan<- []byte) can.HandlerFunc {
	return func(frm can.Frame) {
		bytes := FrameToBytes(frm)
		stream <- bytes

	}
}

func FrameToBytes(frm can.Frame) []byte {
	bytes := make([]byte, 0, 8+frm.Length)
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, frm.ID)

	bytes = append(bytes, out...)
	bytes = append(bytes, frm.Length)
	bytes = append(bytes, frm.Flags)
	bytes = append(bytes, frm.Res0)
	bytes = append(bytes, frm.Res1)
	for _, v := range frm.Data {
		bytes = append(bytes, v)
	}
	return bytes
}
