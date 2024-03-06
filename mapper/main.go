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

func process[T nanomsg.Message](subscriber *nanomsg.Subscriber[T], publisher *nanomsg.Publisher[message.Mapped], mapper RealMapper[T], ignoreEmptyUpdates bool) {
	receiveBuffer := make(chan *T, bufferCapacity)
	defer close(receiveBuffer)
	sendBuffer := make(chan *message.Mapped, bufferCapacity)
	defer close(sendBuffer)

	go subscriber.Receive(receiveBuffer)
	go publisher.Send(sendBuffer)

	var err error

	for in := range receiveBuffer {
		var out *message.Mapped
		if out, err = mapper.DoMap(in); err != nil {
			logger.GetLogger().Warn(
				"Could not map the received data",
				zap.Any("Input", in),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if len(out.Updates) == 0 {
			if !ignoreEmptyUpdates {
				logger.GetLogger().Warn(
					"No updates after mapping the data",
					zap.Any("Input", in),
					zap.Any("Output", out),
				)
			}
			continue
		}
		sendBuffer <- out
	}
}
