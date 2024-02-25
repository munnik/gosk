package nanomsg

import (
	"sync"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"

	// register transports
	"go.nanomsg.org/mangos/v3"
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

// Proxy is a proxy which can subscribe to multiple sockets and publish to a single socket
type Proxy struct {
	publisher    mangos.Socket
	stopChannels []chan struct{}
}

// NewProxy creates a new instance
func NewProxy(url string) *Proxy {
	// don't care about the message type, only the internal socket is used
	p := NewPublisher[message.Raw](url)
	return &Proxy{publisher: p.socket}
}

// SubscribeTo a publisher
func (p *Proxy) SubscribeTo(url string, wg *sync.WaitGroup) {
	stopChannel := make(chan struct{})
	p.stopChannels = append(p.stopChannels, stopChannel)
	topic := []byte("")
	// don't care about the message type, only the internal socket is used
	s, err := NewSubscriber[message.Raw](url, topic)
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not subscribe",
			zap.String("URL", url),
			zap.ByteString("Topic", topic),
			zap.String("Error", err.Error()),
		)
	}
	socket := s.socket
	go func(url string, topic []byte) {
		defer socket.Close()
		for {
			select {
			default:
				if received, err := socket.Recv(); err != nil {
					logger.GetLogger().Warn(
						"Could not receive a message from the publisher",
						zap.String("Error", err.Error()),
					)
				} else {
					if err := p.publisher.Send(received); err != nil {
						logger.GetLogger().Warn(
							"Unable to send the message using NanoMSG",
							zap.ByteString("Message", received),
							zap.String("Error", err.Error()),
						)
						continue
					}
				}
			case <-stopChannel:
				wg.Done()
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
