package nanomsg

import (
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.uber.org/zap"
)

type Message interface {
	message.Raw | message.Mapped
}

func warnBufferSize[T any](buffer chan T, name string) {
	var fillPercentage int
	c := cap(buffer)
	timer := time.NewTicker(10 * time.Millisecond)
	for {
		<-timer.C
		if fillPercentage = (100 * len(buffer)) / c; fillPercentage > 25 {
			logger.GetLogger().Warn(
				"Buffer stats",
				zap.String("buffer", name),
				zap.Int("fill %", fillPercentage),
			)
		}
	}
}
