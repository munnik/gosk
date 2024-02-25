package nanomsg

import (
	"encoding/json"

	"github.com/munnik/gosk/logger"
	"github.com/prometheus/client_golang/prometheus"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/pub"
	"go.uber.org/zap"

	// register transports
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

type Publisher[T Message] struct {
	socket mangos.Socket

	messagesReceivedFromSubscription prometheus.Counter
	messagesMarshalled               prometheus.Counter
	messagesPublished                prometheus.Counter
}

type PublisherOption[T Message] func(*Publisher[T])

func NewPublisher[T Message](url string, opts ...PublisherOption[T]) *Publisher[T] {
	socket, err := pub.NewSocket()
	if err != nil {
		logger.GetLogger().Fatal(
			"Could not create publisher",
			zap.String("Error", err.Error()),
		)
	}
	if err := socket.Listen(url); err != nil {
		logger.GetLogger().Fatal(
			"Could not listen on the URL",
			zap.String("URL", url),
			zap.String("Error", err.Error()),
		)
	}
	result := &Publisher[T]{socket: socket}
	for _, o := range opts {
		o(result)
	}
	return result
}

func (p *Publisher[T]) send(bytes []byte) {
	if err := p.socket.Send(bytes); err != nil {
		logger.GetLogger().Warn(
			"Unable to send the message using NanoMSG",
			zap.ByteString("Message", bytes),
			zap.String("Error", err.Error()),
		)
		return
	}
	if p.messagesPublished != nil {
		p.messagesPublished.Inc()
	}
}

func (p *Publisher[T]) Send(buffer chan *T) {
	go warnBufferSize(buffer, "send")

	for m := range buffer {
		if p.messagesReceivedFromSubscription != nil {
			p.messagesReceivedFromSubscription.Inc()
		}
		go func(m *T) {
			var bytes []byte
			var err error
			if bytes, err = json.Marshal(m); err != nil {
				logger.GetLogger().Warn(
					"Could not marshal the mapped data",
					zap.String("Error", err.Error()),
				)
				return
			}
			if p.messagesMarshalled != nil {
				p.messagesMarshalled.Inc()
			}
			p.send(bytes)
		}(m)
	}
}
