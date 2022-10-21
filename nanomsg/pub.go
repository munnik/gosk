package nanomsg

import (
	"encoding/json"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/pub"
	"go.uber.org/zap"

	// register transports
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

// NewPub creates a new publisher socket
func NewPub(url string) mangos.Socket {
	socket, err := pub.NewSocket()
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not create publisher",
			zap.String("Error", err.Error()),
		)
	}
	if err := socket.Listen(url); err != nil {
		logger.GetLogger().Fatal(
			"Could not listen on the URL",
			zap.String("URL", url),
			zap.String("Error", err.Error()),
		)
	}
	return socket
}

func SendRaw(m *message.Raw, s mangos.Socket) {
	bytes, err := json.Marshal(m)
	if err != nil {
		logger.GetLogger().Warn(
			"Unable to marshall the message to JSON",
			zap.ByteString("Message", m.Value),
			zap.String("Error", err.Error()),
		)
		return
	}
	if err := s.Send(bytes); err != nil {
		logger.GetLogger().Warn(
			"Unable to send the message using NanoMSG",
			zap.ByteString("Message", bytes),
			zap.String("Error", err.Error()),
		)
		return
	}
	logger.GetLogger().Debug(
		"Send the message on the NanoMSG socket",
		zap.ByteString("Message", m.Value),
	)
}

func SendMapped(m *message.Mapped, s mangos.Socket) {
	bytes, err := json.Marshal(m)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not marshal the mapped data",
			zap.String("Error", err.Error()),
		)
		return
	}
	if err := s.Send(bytes); err != nil {
		logger.GetLogger().Warn(
			"Unable to send the message using NanoMSG",
			zap.ByteString("Message", bytes),
			zap.String("Error", err.Error()),
		)
		return
	}

}
