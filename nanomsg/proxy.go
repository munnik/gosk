package nanomsg

import (
	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"

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
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", url),
			zap.ByteString("Topic", topic),
			zap.String("Error", err.Error()),
		)
	}
	go func(url string, topic []byte) {
		defer socket.Close()
		for {
			select {
			default:
				if msg, err := socket.Recv(); err != nil {
					logger.GetLogger().Warn(
						"Error occured when receiving a message",
					)
				} else {
					p.publisher.Send(msg)
				}
			case <-stopChannel:
				return
			}
		}
	}(url, topic)
}

// Close stops and removes all subscribers
func (p *Proxy) Close() {
	for _, stopChannel := range p.stopChannels {
		close(stopChannel)
	}
	p.stopChannels = nil
	p.publisher.Close()
}
