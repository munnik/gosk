package nanomsg

import (
	"encoding/json"

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

	messagesReceivedFromSubscription prometheus.Counter
	messagesUnmarshalled             prometheus.Counter
}

type SubscriberOption[T Message] func(*Subscriber[T])

func NewSubscriber[T Message](url string, topic []byte, opts ...SubscriberOption[T]) (*Subscriber[T], error) {
	socket, err := sub.NewSocket()
	if err != nil {
		return nil, err
	}
	if err := socket.Dial(url); err != nil {
		return nil, err
	}
	socket.SetOption(mangos.OptionSubscribe, topic)
	result := &Subscriber[T]{socket: socket}
	for _, o := range opts {
		o(result)
	}
	return result, nil
}

func (s *Subscriber[T]) receive(buffer chan []byte) {
	go warnBufferSize(buffer, "receive")

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
			if s.messagesReceivedFromSubscription != nil {
				s.messagesReceivedFromSubscription.Inc()
			}
		default:
			go logger.GetLogger().Warn("Buffer is full, dropping received data")
		}
	}
}

func (s *Subscriber[T]) Receive(buffer chan *T) {
	bytesBuffer := make(chan []byte, cap(buffer))
	go s.receive(bytesBuffer)

	message := new(T)
	for bytes := range bytesBuffer {
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
			if s.messagesUnmarshalled != nil {
				s.messagesUnmarshalled.Inc()
			}
		default:
			go logger.GetLogger().Warn("Buffer is full, dropping unmarshalled data")
		}
	}
}
