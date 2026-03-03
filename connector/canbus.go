package connector

import (
	"context"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"

	"go.einride.tech/can/pkg/socketcan"
	"go.uber.org/zap"
)

type CanBusConnector struct {
	config  *config.ConnectorConfig
	timeout *time.Timer
}

func NewCanBusConnector(c *config.ConnectorConfig) (*CanBusConnector, error) {
	return &CanBusConnector{
		config:  c,
		timeout: time.AfterFunc(c.Timeout, exit),
	}, nil
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
	conn, err := socketcan.DialContext(context.Background(), "can", r.config.URL.Host)
	if err != nil {
		return err
	}

	recv := socketcan.NewReceiver(conn)
	for recv.Receive() {
		stream <- []byte(recv.Frame().JSON())
	}
	return nil
}
