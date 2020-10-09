package nanomsg

import (
	"log"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/pub"
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
