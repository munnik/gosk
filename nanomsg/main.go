package nanomsg

import (
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type Message interface {
	message.Raw | message.Mapped
}

func checkBufferSize[T any](buffer chan T, name string, g prometheus.Gauge) {
	var fillPercentage, lastFillPercentage int
	c := cap(buffer)
	timer := time.NewTicker(10 * time.Millisecond)
	for {
		<-timer.C

		// write to log
		if fillPercentage = (100 * len(buffer)) / c; fillPercentage > 25 {
			logger.GetLogger().Warn(
				"Buffer stats",
				zap.String("buffer", name),
				zap.Int("fill %", fillPercentage),
			)
		}

		// write to prometheus
		if g != nil && lastFillPercentage != fillPercentage {
			g.Set(float64(fillPercentage))
			lastFillPercentage = fillPercentage
		}
	}
}
