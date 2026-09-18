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

// Send marshals and publishes everything arriving on buffer, in the order
// it arrives, until buffer is closed.
//
// Marshal-and-send runs inline rather than in a goroutine per message. A
// goroutine per message lets the scheduler decide which message reaches
// socket.Send first, so the publisher can reorder its own output - and
// this is load-bearing further down the pipeline, not just a cosmetic
// ordering nicety: a mapper that keeps "the latest value" state across
// messages (e.g. AggregateMapper.DoMap's env[path] = svm, or
// ModbusMapper.DoMap's delta/rate tracking) has no way to tell a
// reordered message from a genuinely new one, so an older message
// arriving after a newer one silently clobbers it - and a history buffer
// appended to in arrival order, rather than timestamp order, can feed a
// moving average or rate calculation entries out of chronological
// sequence. There is no throughput argument for parallelizing this
// either: mangos' socket.Send only copies into the per-pipe send queues
// and returns, it never blocks on the network - see send below.
func (p *Publisher[T]) Send(buffer chan *T) {
	go checkBufferSize(buffer, "send", p.bufferSizeGauge)

	// One encode buffer, owned by this loop and reused across sends.
	// mangos' socket.Send copies its argument before returning (see
	// go.nanomsg.org/mangos/v3/internal/core/socket.go's Send), so the
	// next iteration is free to overwrite it.
	buf := make([]byte, 0, 512)

	for m := range buffer {
		if p.receivedCounter != nil {
			p.receivedCounter.Inc()
		}

		bytes, err := any(m).(msgp.Marshaler).MarshalMsg(buf[:0])
		if err != nil {
			logger.GetLogger().Warn(
				"Could not marshal the mapped data",
				zap.String("Error", err.Error()),
			)
			continue
		}
		// keep whatever MarshalMsg grew the buffer to, so the next
		// message reuses the larger allocation instead of regrowing it
		buf = bytes

		if p.marshalledCounter != nil {
			p.marshalledCounter.Inc()
		}
		p.send(bytes)
	}
}
