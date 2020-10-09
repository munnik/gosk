package nanomsg

import (
	"log"

	"go.nanomsg.org/mangos/v3"
)

// PubSubProxy is a proxy which can subscribe to multiple sockets and publish to a single socket
type PubSubProxy struct {
	stopChannels []chan struct{}
	publisher    mangos.Socket
}

// NewPubSubProxy creates a new instance
func NewPubSubProxy(url string) PubSubProxy {
	return PubSubProxy{publisher: NewPub(url)}
}

// AddSubscriber adds a new subscriber
func (p PubSubProxy) AddSubscriber(url string, topic []byte) {
	stopChannel := make(chan struct{})
	p.stopChannels = append(p.stopChannels, stopChannel)
	socket := NewSub(url, topic)
	go func(subscriber mangos.Socket) {
		defer subscriber.Close()
		for {
			select {
			default:
				if msg, err := subscriber.Recv(); err != nil {
					log.Fatal(err)
				} else {
					p.publisher.Send(msg)
				}
			case <-stopChannel:
				return
			}
		}
	}(socket)
}

// Close stops and removes all subscribers
func (p PubSubProxy) Close() {
	for _, stopChannel := range p.stopChannels {
		close(stopChannel)
	}
	p.stopChannels = nil
	p.publisher.Close()
}
