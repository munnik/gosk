package connector

import (
	"fmt"
	"net"
	"net/url"
)

// listener keeps the socket a connector listens on (config.Listen) for the
// whole life of that connector, and hands out the connections a sensor
// makes to it.
//
// The connectors that support listening used to call net.Listen for every
// connection and then drop the listener without ever closing it. That works
// exactly once: the next connection finds the port still bound by the
// listener nobody closed, fails with "address already in use", and keeps
// failing every retry - so a sensor that disconnects once can never come
// back without the unit being restarted, however healthy it is. Holding on
// to the listener and accepting again also keeps the port bound in between,
// so there is no window in which the sensor's own reconnect is refused.
//
// Not safe for concurrent use: both connectors dial from the single
// goroutine Publish starts for it.
type listener struct {
	socket net.Listener
}

// accept returns the next connection made to u, listening on it first if
// this is the first call.
func (l *listener) accept(u *url.URL) (net.Conn, error) {
	address := net.JoinHostPort(u.Hostname(), u.Port())
	if l.socket == nil {
		socket, err := net.Listen(u.Scheme, address)
		if err != nil {
			return nil, fmt.Errorf("unable to listen on %v, the error that occurred was %v", u.String(), err)
		}
		l.socket = socket
	}
	conn, err := l.socket.Accept()
	if err != nil {
		return nil, fmt.Errorf("unable to accept a connection on %v, the error that occurred was %v", u.String(), err)
	}
	return conn, nil
}
