package mapper

import (
	"encoding/json"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// Mapper interface
type Mapper interface {
	Map(subscriber mangos.Socket, publisher mangos.Socket)
}

type RealMapper interface {
	DoMap(*message.Raw) (*message.Mapped, error)
}

func process(subscriber mangos.Socket, publisher mangos.Socket, mapper RealMapper) {
	raw := &message.Raw{}
	var mapped *message.Mapped
	var toSend []byte
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, raw); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if mapped, err = mapper.DoMap(raw); err != nil {
			logger.GetLogger().Warn(
				"Could not map the received data",
				zap.ByteString("Raw bytes", raw.Value),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if toSend, err = json.Marshal(mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not marshal the mapped data",
				zap.String("Error", err.Error()),
			)
			continue
		}
		publisher.Send(toSend)
	}
}
