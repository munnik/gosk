package nanomsg

import (
	"log"

	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/sub"
)

// NewSub creates a new subscriber socket
func NewSub(url string, topic []byte) mangos.Socket {
	socket, err := sub.NewSocket()
	if err != nil {
		log.Fatalf("Can't create subSocket: %s", err)
	}
	if err := socket.Dial(url); err != nil {
		log.Fatal(err)
	}
	socket.SetOption(mangos.OptionSubscribe, topic)
	return socket
}
