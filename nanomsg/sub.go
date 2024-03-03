package nanomsg

import (
	"encoding/json"
	"sync"

	"github.com/munnik/gosk/logger"
	"github.com/prometheus/client_golang/prometheus"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/sub"
	"go.uber.org/zap"

	// register transports
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

type Subscriber[T Message] struct {
	socket mangos.Socket
	pool   *sync.Pool

	receivedCounter     prometheus.Counter
	unmarshalledCounter prometheus.Counter
	bufferSizeGauge     prometheus.Gauge
}

type SubscriberOption[T Message] func(*Subscriber[T])

func WithSubscriberReceivedCounter[T Message](c prometheus.Counter) SubscriberOption[T] {
	return func(s *Subscriber[T]) {
		s.receivedCounter = c
	}
}
func WithSubscriberUnmarshalledCounter[T Message](c prometheus.Counter) SubscriberOption[T] {
	return func(s *Subscriber[T]) {
		s.unmarshalledCounter = c
	}
}

func WithSubscriberBufferSizeGauge[T Message](g prometheus.Gauge) SubscriberOption[T] {
	return func(s *Subscriber[T]) {
		s.bufferSizeGauge = g
	}
}

func NewSubscriber[T Message](url string, topic []byte, opts ...SubscriberOption[T]) (*Subscriber[T], error) {
	socket, err := sub.NewSocket()
	if err != nil {
		return nil, err
	}
	if err := socket.Dial(url); err != nil {
		return nil, err
	}
	socket.SetOption(mangos.OptionSubscribe, topic)

	pool := &sync.Pool{
		New: func() any {
			return new(T)
		},
	}

	result := &Subscriber[T]{socket: socket, pool: pool}
	for _, o := range opts {
		o(result)
	}
	return result, nil
}

func (s *Subscriber[T]) receive(buffer chan []byte) {
	go checkBufferSize(buffer, "receive", s.bufferSizeGauge)

	for {
		received, err := s.socket.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		select {
		case buffer <- received:
			if s.receivedCounter != nil {
				s.receivedCounter.Inc()
			}
		default:
			go logger.GetLogger().Warn("Buffer is full, dropping received data")
		}
	}
}

func (s *Subscriber[T]) Receive(buffer chan *T) {
	bytesBuffer := make(chan []byte, cap(buffer))
	go s.receive(bytesBuffer)

	for bytes := range bytesBuffer {
		message := s.pool.Get().(*T)
		if err := json.Unmarshal(bytes, message); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
		select {
		case buffer <- message:
			if s.unmarshalledCounter != nil {
				s.unmarshalledCounter.Inc()
			}
		default:
			go logger.GetLogger().Warn("Buffer is full, dropping unmarshalled data")
		}
		s.pool.Put(message)
	}
}
