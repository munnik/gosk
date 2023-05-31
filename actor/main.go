package actor

import (
	"encoding/json"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type Actor interface {
	Act(subscriber mangos.Socket, publisher mangos.Socket)
}

type RealActor interface {
	DoAct(*message.ActionRequest) (*message.Raw, *message.ActionResponse)
}

func process(subscriber mangos.Socket, publisher mangos.Socket, actor RealActor) {
	actionRequestMessage := &message.ActionRequest{}
	var rawMessage *message.Raw
	var actionResponseMessage *message.ActionResponse
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
		if err := json.Unmarshal(received, actionRequestMessage); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if rawMessage, actionResponseMessage = actor.DoAct(actionRequestMessage); actionResponseMessage.StatusCode != message.STATUS_CODE_SUCCESSFUL {
			logger.GetLogger().Warn(
				"Could not map the received data",
				zap.String("Path", actionRequestMessage.Put.Path),
				zap.Any("Value", actionRequestMessage.Put.Value),
				zap.Int("StatusCode", actionResponseMessage.StatusCode),
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
