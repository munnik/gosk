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
	config *config.ConnectorConfig
}

// NewCanBusConnector does not open the CAN interface - receive does, from
// the goroutine Publish starts, so that process is already running and
// reporting DisconnectedOrNoData while the interface is still absent. See
// NewLineConnector for what opening it up front cost.
func NewCanBusConnector(c *config.ConnectorConfig) (*CanBusConnector, error) {
	return &CanBusConnector{
		config: c,
	}, nil
}

func (r *CanBusConnector) Publish(publisher *nanomsg.Publisher[message.Raw]) {
	stream := make(chan []byte, 1)
	defer close(stream)
	go func() {
		// A CAN interface that does not exist - the transceiver is not
		// plugged in, or the link was never brought up - fails to dial
		// instantly, so retrying without waiting pinned a core and wrote
		// the same warning thousands of times a second. Wait between
		// attempts, and only say why when the reason changes; the repeats
		// go to debug. Mirrors LineConnector.createConnection.
		var lastError string
		for {
			if err := r.receive(stream); err != nil {
				if err.Error() == lastError {
					logger.GetLogger().Debug(
						"Error while receiving data for the stream, retrying",
						zap.String("URL", r.config.URL.String()),
						zap.Duration("Retrying in", retryConnectionInterval),
						zap.String("Error", err.Error()),
					)
				} else {
					logger.GetLogger().Warn(
						"Error while receiving data for the stream, retrying",
						zap.String("URL", r.config.URL.String()),
						zap.Duration("Retrying in", retryConnectionInterval),
						zap.String("Error", err.Error()),
					)
					lastError = err.Error()
				}
			} else {
				// The interface was there and then stopped yielding
				// frames, which is worth saying once per occurrence.
				logger.GetLogger().Warn(
					"The stream ended, reconnecting",
					zap.String("URL", r.config.URL.String()),
					zap.Duration("Retrying in", retryConnectionInterval),
				)
				lastError = ""
			}
			time.Sleep(retryConnectionInterval)
		}
	}()
	process(stream, r.config.Name, r.config.Protocol, publisher, r.config.Timeout)
}

func (*CanBusConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	// do nothing
}

func (r *CanBusConnector) receive(stream chan<- []byte) error {
	conn, err := socketcan.DialContext(context.Background(), "can", r.config.URL.Host)
	if err != nil {
		return err
	}
	// Publish redials on every return from here, so the socket this one
	// opened has to go with it - it used to be left behind, leaking a file
	// descriptor per attempt until the process hit its limit and could no
	// longer open anything at all.
	defer conn.Close()

	recv := socketcan.NewReceiver(conn)
	for recv.Receive() {
		stream <- []byte(recv.Frame().JSON())
	}
	// Receive returning false is either a read error or a closed socket;
	// Err distinguishes them, and returning it is what gets the reason for
	// a vanished interface into the journal instead of a bare reconnect.
	return recv.Err()
}
