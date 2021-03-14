package nanomsg

import (
	log "github.com/sirupsen/logrus"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/pub"

	// register transports
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

// NewPub creates a new publisher socket
func NewPub(url string) mangos.Socket {
	socket, err := pub.NewSocket()
	if err != nil {
		log.Fatalf("Can't create pubSocket: %s", err)
	}
	if err := socket.Listen(url); err != nil {
		log.Fatal(err)
	}
	return socket
}
