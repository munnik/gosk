package mapper

import (
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
// Readiness (see sdnotify.Ready) is signalled here directly, as soon as
// this mapper is subscribed and publishing, rather than left to fire
// implicitly via publisher's first successful send (see nanomsg/pub.go's
// send). A mapper fed real but permanently undecodable input - e.g. an
// NMEA0183 sentence type this mapper has no case for - would otherwise
// never publish anything at all and so never become ready, even though it
// is correctly doing its job: this was observed live on
// node-deme-grinza6's mapEchoSounder, stuck restarting under
// TimeoutStartSec forever on a stream of unsupported $GPBWC sentences.
//
// It does not wait for the first message to arrive either, which it used
// to. That only moved the coupling one hop upstream: with a sensor that is
// absent, the first message a mapper ever receives is its connector's
// DisconnectedOrNoData report, which arrives a whole
// config.ConnectorConfig.Timeout after the connector started (30s by
// default, against gosk.nix's 40s TimeoutStartSec) and repeats on that
// interval - so raising a connector's timeout past TimeoutStartSec started
// the connect unit fine but failed every map and notify unit behind it,
// and each further stage in a chain could add another timeout's wait of
// its own, since nanomsg pub/sub has no replay for a subscriber that
// connected after a report was published. Whether anything upstream has
// something to say is not this process's own health.
func process[T nanomsg.Message](subscriber *nanomsg.Subscriber[T], publisher *nanomsg.Publisher[message.Mapped], mapper RealMapper[T], ignoreEmptyUpdates bool) {
	receiveBuffer := make(chan *T, bufferSize)
	sendBuffer := make(chan *message.Mapped, bufferSize)
	defer close(sendBuffer)

	go subscriber.Receive(receiveBuffer)
	go publisher.Send(sendBuffer)
	sdnotify.Ready()

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
					out := csm.MapConnectorStatus(raw.Connector, string(raw.Value) == message.ConnectorStatusConnectedAndData)
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
// reasons - see process's doc comment.
func processRaw[T nanomsg.Message](subscriber *nanomsg.Subscriber[T], publisher *nanomsg.Publisher[message.Raw], mapper RealRawMapper[T]) {
	receiveBuffer := make(chan *T, bufferSize)
	sendBuffer := make(chan *message.Raw, bufferSize)
	defer close(sendBuffer)

	go subscriber.Receive(receiveBuffer)
	go publisher.Send(sendBuffer)
	sdnotify.Ready()

	var err error

	for in := range receiveBuffer {
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
