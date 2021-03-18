package nanomsg

import (
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
		Logger.Fatal(
			"Could not create publisher",
			zap.String("Error", err.Error()),
		)
	}
	if err := socket.Listen(url); err != nil {
		Logger.Fatal(
			"Could not listen on the URL",
			zap.String("URL", url),
			zap.String("Error", err.Error()),
		)
	}
	return socket
}
