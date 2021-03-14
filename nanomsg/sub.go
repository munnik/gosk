package nanomsg

import (
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/sub"

	// register transports
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

// NewSub creates a new subscriber socket
func NewSub(url string, topic []byte) (mangos.Socket, error) {
	socket, err := sub.NewSocket()
	if err != nil {
		return nil, err
	}
	if err := socket.Dial(url); err != nil {
		return nil, err
	}
	socket.SetOption(mangos.OptionSubscribe, topic)
	return socket, nil
}
