package nanomsg

import (
	"fmt"

	"github.com/munnik/gosk/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinylib/msgp/msgp"
	"go.nanomsg.org/mangos/v3"
	"go.nanomsg.org/mangos/v3/protocol/sub"
	"go.uber.org/zap"

	// register transports
	_ "go.nanomsg.org/mangos/v3/transport/all"
)

type Subscriber[T Message] struct {
	socket mangos.Socket

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

// NewSubscriber subscribes to every publisher in urls at once. A single
// nanomsg sub socket can dial any number of endpoints and multiplexes all
// of them into one Recv stream, which is exactly what the separate proxy
// process used to do by hand: fan several publishers into one subscriber.
// Doing it here instead means a processor names the processors it reads
// from directly, with no extra process, no extra hop and no extra copy of
// every message in between.
//
// The endpoints are dialled asynchronously, so a publisher that is not
// listening yet - or not any more - neither fails the subscription nor
// holds up the endpoints next to it, mangos keeps retrying it in the
// background and redials it when it comes back. This replaces the retry
// loop that used to sit here: with one url it blocked until that
// publisher was up, which is exactly the wrong behaviour once there are
// several, where one publisher being down would otherwise stop the
// subscriber from ever reading the others.
func NewSubscriber[T Message](urls []string, topic []byte, opts ...SubscriberOption[T]) (*Subscriber[T], error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no url to subscribe to")
	}

	socket, err := sub.NewSocket()
	if err != nil {
		return nil, err
	}

	for _, url := range urls {
		// an error here is the url itself being unusable (malformed, or
		// an unknown transport), not the publisher being unreachable,
		// so it is worth failing on rather than retrying
		if err := socket.DialOptions(url, map[string]interface{}{mangos.OptionDialAsynch: true}); err != nil {
			return nil, fmt.Errorf("could not dial the publisher %v: %w", url, err)
		}
	}
	if err := socket.SetOption(mangos.OptionSubscribe, topic); err != nil {
		return nil, err
	}

	result := &Subscriber[T]{socket: socket}
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
	receiveBuffer := make(chan []byte, cap(buffer))
	go s.receive(receiveBuffer)

	for bytes := range receiveBuffer {
		m := new(T)
		if _, err := any(m).(msgp.Unmarshaler).UnmarshalMsg(bytes); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
		select {
		case buffer <- m:
			if s.unmarshalledCounter != nil {
				s.unmarshalledCounter.Inc()
			}
		default:
			go logger.GetLogger().Warn("Buffer is full, dropping unmarshalled data")
		}
	}
}
