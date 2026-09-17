package nanomsg

import (
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// Message is deliberately just the type union, not also a method-set
// requirement: Mapped/Raw's MarshalMsg has a value receiver but
// UnmarshalMsg has a pointer one (it has to, to mutate the receiver) -
// Go's generics can't express "T's pointer type has this method" as part
// of a plain type-union constraint. Publisher.Send and Subscriber.Receive
// instead reach msgp.Marshaler/Unmarshaler via a runtime type assertion
// on *T, exactly the way encoding/json.Marshal/Unmarshal already dispatch
// to a type's MarshalJSON/UnmarshalJSON internally - not a new pattern,
// just moved from the stdlib's own reflection into ours.
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
