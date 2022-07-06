package collector

import (
	"encoding/binary"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"

	"github.com/brutella/can"
)

type CanBusCollector struct {
	config *config.CollectorConfig
}

func NewCanBusCollector(c *config.CollectorConfig) (*CanBusCollector, error) {
	return &CanBusCollector{config: c}, nil
}

func (r *CanBusCollector) Collect(publisher mangos.Socket) {
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
	process(stream, r.config.Name, r.config.Protocol, publisher)
}

func (r *CanBusCollector) receive(stream chan<- []byte) error {
	bus, err := can.NewBusForInterfaceWithName(r.config.URL.Host)
	if err != nil {
		return err
	}
	defer bus.Disconnect()
	bus.SubscribeFunc(handleCanFrameStream(stream))
	bus.ConnectAndPublish()
	return nil
}

func handleCanFrameStream(stream chan<- []byte) can.HandlerFunc {
	return func(frm can.Frame) {
		// fmt.Println(frm)
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
