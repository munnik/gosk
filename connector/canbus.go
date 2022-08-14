package connector

import (
	"encoding/binary"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"

	"github.com/brutella/can"
)

type CanBusConnector struct {
	config *config.ConnectorConfig
}

func NewCanBusConnector(c *config.ConnectorConfig) (*CanBusConnector, error) {
	return &CanBusConnector{config: c}, nil
}

func (r *CanBusConnector) Connect(publisher mangos.Socket, subscriber mangos.Socket) {
	// write data to the connection
	go func() {
		for {
			subscriber.Recv()
			// TODO: implement writes to canbus
			logger.GetLogger().Error("Writing to canbus is not implemented yet")
		}
	}()

	// receive data from the connection
	c := make(chan []byte, receiveChannelBufferSize)
	defer close(c)
	go func() {
		for {
			if err := r.receive(c); err != nil {
				logger.GetLogger().Warn(
					"Error while receiving data for the stream",
					zap.String("URL", r.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
	process(c, r.config.Name, r.config.Protocol, publisher)
}

func (r *CanBusConnector) receive(c chan<- []byte) error {
	bus, err := can.NewBusForInterfaceWithName(r.config.URL.Host)
	if err != nil {
		return err
	}
	defer bus.Disconnect()
	bus.SubscribeFunc(handleCanFrameStream(c))
	bus.ConnectAndPublish()
	return nil
}

func handleCanFrameStream(c chan<- []byte) can.HandlerFunc {
	return func(frm can.Frame) {
		bytes := FrameToBytes(frm)
		c <- bytes
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
