package nanomsg

import (
	"sync"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/sdnotify"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinylib/msgp/msgp"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/pub"
	"go.uber.org/zap"

	// register transports
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

// msgBufferPool holds reusable msgpack encode buffers for Send. mangos'
// socket.Send copies its argument before returning (see
// go.nanomsg.org/mangos/v3/internal/core/socket.go's Send), so a buffer is
// safe to return to the pool as soon as send() below is done with it.
var msgBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

type Publisher[T Message] struct {
	socket mangos.Socket
	// ready fires sdnotify.Ready once, the first time this publisher
	// successfully sends a message - see its doc comment for why "has
	// published something real" is a more meaningful readiness signal
	// than "the socket is bound", which happens moments after process
	// start regardless of whether anything is actually flowing yet.
	ready sync.Once

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
	p.ready.Do(sdnotify.Ready)
}

func (p *Publisher[T]) Send(buffer chan *T) {
	go checkBufferSize(buffer, "send", p.bufferSizeGauge)

	for m := range buffer {
		if p.receivedCounter != nil {
			p.receivedCounter.Inc()
		}
		go func(m *T) {
			bufPtr := msgBufferPool.Get().(*[]byte)
			defer msgBufferPool.Put(bufPtr)

			bytes, err := any(m).(msgp.Marshaler).MarshalMsg((*bufPtr)[:0])
			if err != nil {
				logger.GetLogger().Warn(
					"Could not marshal the mapped data",
					zap.String("Error", err.Error()),
				)
				return
			}
			*bufPtr = bytes
			if p.marshalledCounter != nil {
				p.marshalledCounter.Inc()
			}
			p.send(bytes)
		}(m)
	}
}
