package mapper

import (
	"sync"
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/sdnotify"
	"go.uber.org/zap"
)

const bufferSize = 1 << 16

// Mapper interface
type Mapper[TS nanomsg.Message, TP nanomsg.Message] interface {
	Map(subscriber *nanomsg.Subscriber[TS], publisher *nanomsg.Publisher[TP])
}

type RealMapper[T nanomsg.Message] interface {
	DoMap(*T) (*message.Mapped, error)
	GetTickerInterval() time.Duration
}

type RealRawMapper[T nanomsg.Message] interface {
	DoMap(*T) (*message.Raw, error)
}

// periodicMapper is implemented by mappers that can additionally be
// re-evaluated on a fixed interval instead of only reacting to incoming
// data, see process. refreshMap is called with the current time and returns
// the resulting output, if any.
type periodicMapper interface {
	refreshMap(now time.Time) *message.Mapped
}

// process runs mapper reactively: every value received from subscriber is
// passed to mapper.DoMap and the result, if non-empty, is published.
//
// When interval is positive and mapper also implements periodicMapper,
// mapper.refreshMap is additionally called on that interval, and its result
// published the same way. This lets a mapper notice something an incoming
// value alone would not, e.g. that expected data has stopped arriving
// entirely. interval is ignored, without effect, for a mapper that does not
// implement periodicMapper, and a zero interval (the default) never starts
// the ticker at all, so mappers that do not opt in behave exactly as
// before.
//
// Readiness (see sdnotify.Ready) is signalled here directly, on the first
// message consumed from receiveBuffer, rather than left to fire implicitly
// via publisher's first successful send (see nanomsg/pub.go's send) as
// every other processor type does. A mapper fed real but permanently
// undecodable input - e.g. an NMEA0183 sentence type this mapper has no
// case for - would otherwise never publish anything at all and so never
// become ready, even though it is correctly doing its job: this was
// observed live on node-deme-grinza6's mapEchoSounder, stuck restarting
// under TimeoutStartSec forever on a stream of unsupported $GPBWC
// sentences. Consuming a connector's ConnectorStatusType report doesn't by
// itself guarantee ready fires either - the connector only sends
// ConnectedAndData once, on its disconnected->connected transition, and if
// that happens before this mapper's subscriber has connected, nanomsg's
// lossy pub/sub simply drops it with no replay for a late subscriber - so
// readiness can't safely be left to depend on that message either.
func process[T nanomsg.Message](subscriber *nanomsg.Subscriber[T], publisher *nanomsg.Publisher[message.Mapped], mapper RealMapper[T], ignoreEmptyUpdates bool) {
	receiveBuffer := make(chan *T, bufferSize)
	sendBuffer := make(chan *message.Mapped, bufferSize)
	defer close(sendBuffer)

	go subscriber.Receive(receiveBuffer)
	go publisher.Send(sendBuffer)

	var ready sync.Once

	// tick is left nil, and is therefore never selected, unless the mapper
	// implements periodicMapper and was configured with a positive interval
	var tick <-chan time.Time
	sweeper, canSweep := mapper.(periodicMapper)
	tickerInterval := mapper.GetTickerInterval()
	if canSweep && tickerInterval > 0 {
		ticker := time.NewTicker(tickerInterval)
		defer ticker.Stop()
		tick = ticker.C
	}

	for {
		select {
		case in, ok := <-receiveBuffer:
			if !ok {
				return
			}
			ready.Do(sdnotify.Ready)
			// A connector status report (see message.ConnectorStatusType)
			// isn't protocol data - DoMap doesn't know how to decode it,
			// and shouldn't have to. Hand it to MapConnectorStatus instead,
			// when the mapper implements it; a mapper that doesn't is
			// presumably not fed by a connector's Raw stream at all (e.g.
			// AggregateMapper, NotificationMapper), so any T for which
			// this type assertion could even succeed already implements
			// it in practice.
			if raw, ok := any(*in).(message.Raw); ok && raw.Type == message.ConnectorStatusType {
				if csm, ok := mapper.(ConnectorStatusMapper); ok {
					out := csm.MapConnectorStatus(raw.Connector, string(raw.Value) == message.ConnectorStatusConnectedAndData, raw.Uuid)
					if len(out.Updates) > 0 {
						sendBuffer <- out
					}
				}
				continue
			}
			out, err := mapper.DoMap(in)
			if err != nil {
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
		case now := <-tick:
			if out := sweeper.refreshMap(now); len(out.Updates) > 0 {
				sendBuffer <- out
			}
		}
	}
}

// processRaw signals readiness the same way process does, and for the same
// reason - see process's doc comment.
func processRaw[T nanomsg.Message](subscriber *nanomsg.Subscriber[T], publisher *nanomsg.Publisher[message.Raw], mapper RealRawMapper[T]) {
	receiveBuffer := make(chan *T, bufferSize)
	sendBuffer := make(chan *message.Raw, bufferSize)
	defer close(sendBuffer)

	go subscriber.Receive(receiveBuffer)
	go publisher.Send(sendBuffer)

	var ready sync.Once
	var err error

	for in := range receiveBuffer {
		ready.Do(sdnotify.Ready)
		var out *message.Raw
		if out, err = mapper.DoMap(in); err != nil {
			logger.GetLogger().Warn(
				"Could not map the received data",
				zap.Any("Input", in),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if out != nil {
			sendBuffer <- out
		}
	}
}
