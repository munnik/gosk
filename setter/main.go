package setter

import (
	"encoding/json"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type Setter interface {
	Set(subscriber mangos.Socket, publisher mangos.Socket)
}

type RealSetter interface {
	DoSet(*message.Set) (*message.Raw, error)
}

func process(subscriber mangos.Socket, publisher mangos.Socket, setter RealSetter) {
	setMessage := &message.Set{}
	var rawMessage *message.Raw
	var bytes []byte
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, setMessage); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if rawMessage, err = setter.DoSet(setMessage); err != nil {
			logger.GetLogger().Warn(
				"Could not map the received data",
				zap.Any("Value", setMessage.Value),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if bytes, err = json.Marshal(rawMessage); err != nil {
			logger.GetLogger().Warn(
				"Could not marshal the mapped data",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := publisher.Send(bytes); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
	}
}
