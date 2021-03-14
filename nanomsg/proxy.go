package nanomsg

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"go.nanomsg.org/mangos/v3"

	// register transports
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

// Proxy is a proxy which can subscribe to multiple sockets and publish to a single socket
type Proxy struct {
	publisher    mangos.Socket
	stopChannels []chan struct{}
}

// NewProxy creates a new instance
func NewProxy(url string) *Proxy {
	return &Proxy{publisher: NewPub(url)}
}

// SubscribeTo a publisher
func (p *Proxy) SubscribeTo(url string) {
	stopChannel := make(chan struct{})
	p.stopChannels = append(p.stopChannels, stopChannel)
	topic := []byte("")
	socket, err := NewSub(url, topic)
	if err != nil {
		log.Fatal(err)
		panic(err)
	}
	go func(url string, topic []byte) {
		defer socket.Close()
		for {
			select {
			default:
				if msg, err := socket.Recv(); err != nil {
					log.Warn(err)
				} else {
					log.Debug(fmt.Sprintf("The proxy received a message from one of the publishers: %s", msg))
					p.publisher.Send(msg)
				}
			case <-stopChannel:
				log.Debug("In stop channel")
				return
			}
		}
	}(url, topic)
}

// Close stops and removes all subscribers
func (p *Proxy) Close() {
	log.Info("Closing all subscribtions")
	for _, stopChannel := range p.stopChannels {
		close(stopChannel)
	}
	p.stopChannels = nil
	log.Info("Closing the publisher")
	p.publisher.Close()
}
