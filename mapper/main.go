package mapper

import (
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

const bufferCapacity = 5000

// Mapper interface
type Mapper[TS nanomsg.Message, TP nanomsg.Message] interface {
	Map(subscriber *nanomsg.Subscriber[TS], publisher *nanomsg.Publisher[TP])
}

type RealMapper[T nanomsg.Message] interface {
	DoMap(*T) (*message.Mapped, error)
}

func process[T nanomsg.Message](subscriber *nanomsg.Subscriber[T], publisher *nanomsg.Publisher[message.Mapped], mapper RealMapper[T]) {
	receiveBuffer := make(chan *T, bufferCapacity)
	sendBuffer := make(chan *message.Mapped, bufferCapacity)
	go subscriber.Receive(receiveBuffer)
	go publisher.Send(sendBuffer)

	// ugly hack to call the right function, are there better options?
	if receiveBufferRaw, ok := any(receiveBuffer).(chan *message.Raw); ok {
		if mapperRaw, ok := any(mapper).(RealMapper[message.Raw]); ok {
			for received := range receiveBufferRaw {
				rawMap(received, mapperRaw, sendBuffer)
			}
		}
	}
	if receiveBufferMapped, ok := any(receiveBuffer).(chan *message.Mapped); ok {
		if mapperMapped, ok := any(mapper).(RealMapper[message.Mapped]); ok {
			for received := range receiveBufferMapped {
				mappedMap(received, mapperMapped, sendBuffer)
			}
		}
	}

	close(receiveBuffer)
	close(sendBuffer)
}

func rawMap(in *message.Raw, mapper RealMapper[message.Raw], sendBuffer chan *message.Mapped) {
	var out *message.Mapped
	var err error
	if out, err = mapper.DoMap(in); err != nil {
		logger.GetLogger().Warn(
			"Could not map the received data",
			zap.ByteString("Raw bytes", in.Value),
			zap.String("Error", err.Error()),
		)
		return
	}
	sendBuffer <- out
}

func mappedMap(in *message.Mapped, mapper RealMapper[message.Mapped], sendBuffer chan *message.Mapped) {
	var out *message.Mapped
	var err error
	if out, err = mapper.DoMap(in); err != nil {
		logger.GetLogger().Warn(
			"Could not map the received data",
			zap.Any("Input data", in),
			zap.String("Error", err.Error()),
		)
		return
	}
	if len(out.Updates) == 0 {
		return // skip the delta
	}
	sendBuffer <- out
}
