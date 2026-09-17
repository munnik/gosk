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

// bufferCheckInterval bounds how often a congested buffer's fill % gets
// logged. At 10ms, a buffer sitting above the 25% warn threshold for any
// stretch of time - the exact situation this warning exists to surface -
// logged up to 100 lines/second, on top of whatever backpressure it was
// already reporting: real, self-inflicted CPU (JSON-encoding and writing
// each line) piled onto a stage that was already the bottleneck. 1s still
// surfaces sustained congestion promptly without amplifying it.
const bufferCheckInterval = 1 * time.Second

func checkBufferSize[T any](buffer chan T, name string, g prometheus.Gauge) {
	var fillPercentage, lastFillPercentage int
	c := cap(buffer)
	timer := time.NewTicker(bufferCheckInterval)
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
