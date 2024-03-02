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

	receivedCounter   prometheus.Counter
	marshalledCounter prometheus.Counter
	publishedCounter  prometheus.Counter
	bufferSizeGauge   prometheus.Gauge
}

type PublisherOption[T Message] func(*Publisher[T])

func WithPublisherReceivedCounter[T Message](c prometheus.Counter) PublisherOption[T] {
	return func(p *Publisher[T]) {
		p.receivedCounter = c
	}
}

func WithPublisherMarshalledCounter[T Message](c prometheus.Counter) PublisherOption[T] {
	return func(p *Publisher[T]) {
		p.marshalledCounter = c
	}
}

func WithPublisherPublishedCounter[T Message](c prometheus.Counter) PublisherOption[T] {
	return func(p *Publisher[T]) {
		p.publishedCounter = c
	}
}

func WithPublisherBufferSizeGauge[T Message](g prometheus.Gauge) PublisherOption[T] {
	return func(p *Publisher[T]) {
		p.bufferSizeGauge = g
	}
}

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
	if p.publishedCounter != nil {
		p.publishedCounter.Inc()
	}
}

func (p *Publisher[T]) Send(buffer chan *T) {
	go checkBufferSize(buffer, "send", p.bufferSizeGauge)

	for m := range buffer {
		if p.receivedCounter != nil {
			p.receivedCounter.Inc()
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
			if p.marshalledCounter != nil {
				p.marshalledCounter.Inc()
			}
			p.send(bytes)
		}(m)
	}
}
