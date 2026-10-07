package connector

import (
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/sdnotify"
	"go.uber.org/zap"
)

const bufferCapacity = 5000

// retryConnectionInterval is how long a connector waits before dialling its
// sensor again, after a dial failed or an established connection ended. A
// sensor that is switched off, unplugged or not wired up yet fails to dial
// immediately rather than after a timeout, so without this wait a redial
// loop is a busy loop: it burns a core and floods the journal for as long
// as the sensor stays absent, which - see process - is a condition gosk is
// expected to sit in indefinitely rather than exit over.
const retryConnectionInterval = 5 * time.Second

// connectedHeartbeatFactor makes process re-announce ConnectedAndData that
// many times less often than it repeats DisconnectedOrNoData - five minutes
// apart at the default timeout.
//
// All the re-announcement has to do is bound how long a stale offline
// notification can survive after a mapper restart (see the heartbeat in
// process), for which minutes are ample. Matching the disconnected interval
// instead would put a message on the bus, a delta on the SignalK server and
// a row in the database every timeout for every connector on every vessel,
// purely to say nothing has changed - and a fleet is almost entirely
// healthy connectors. Repeating the offline report that often costs nothing
// by comparison, because it only happens while something is wrong.
const connectedHeartbeatFactor = 10

// resolveTimeout is the timeout a connector that polls on pollingIntervals
// should hand to process: whatever its config file asked for, or one sized
// from those intervals when it asked for nothing (see
// config.DefaultTimeoutFor). A connector with no configured intervals to go
// on - anything reading a stream rather than polling - passes none, and gets
// config.DefaultConnectorTimeout.
//
// Which it settled on is logged, because it decides when an alarm is raised
// for this sensor, and an operator looking at an alarm that fires too eagerly
// or too late should not have to work out whether a default was involved.
func resolveTimeout(c *config.ConnectorConfig, pollingIntervals []time.Duration) time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	if c.Timeout < 0 {
		logger.GetLogger().Warn(
			"Configured timeout is negative, deriving one instead",
			zap.String("connector", c.Name),
			zap.Duration("Configured", c.Timeout),
		)
	}
	timeout := config.DefaultTimeoutFor(pollingIntervals...)
	logger.GetLogger().Info(
		"No timeout configured, using one derived from how this connector polls",
		zap.String("connector", c.Name),
		zap.Duration("Timeout", timeout),
	)
	return timeout
}

// Connector interface
type Connector[T nanomsg.Message] interface {
	Publish(publisher *nanomsg.Publisher[T])
	Subscribe(subscriber *nanomsg.Subscriber[T])
}

// process publishes everything read from stream, tagged with connector and
// protocol, until stream is closed.
//
// It also tracks whether the stream has gone quiet: if nothing arrives for
// timeoutDuration - the sensor never connected, or stopped responding -
// it publishes a ConnectorStatusType Raw message (ConnectedAndData or
// DisconnectedOrNoData) instead of exiting the process, so a mapper (see
// mapper/main.go's process) can turn this into a SignalK notification.
// This replaced exiting the process (relying on systemd to restart it)
// specifically because that made a connector timing out - which just
// means "the sensor isn't there right now", not a process-level fault -
// look identical to a real crash, and both nixos-rebuild switch and
// deploy-rs treat a unit that never reaches "started" as a failed
// deploy: one offline sensor could fail an entire fleet-wide deployment
// (see gosk.nix's history of exactly this with the FFT mappers, and
// node-deme-grinza6's Ampero TCP module).
//
// The DisconnectedOrNoData report repeats every timeoutDuration for as
// long as the stream stays quiet, rather than firing once, so that a
// mapper which subscribed late still learns the sensor is missing -
// nanomsg pub/sub is lossy and has no replay, see the heartbeat below for
// the same problem in the other direction.
func process(stream <-chan []byte, connector string, protocol string, publisher *nanomsg.Publisher[message.Raw], timeoutDuration time.Duration) {
	// Both timers below are built from timeoutDuration, and NewTicker panics
	// on a non-positive interval - which would take the connector down at
	// startup, exactly the thing this whole status mechanism exists to
	// avoid. A zero timeout is meaningless anyway: it would report
	// DisconnectedOrNoData continuously, as fast as the scheduler allows,
	// however healthy the sensor is.
	//
	// Every connector resolves its timeout before calling this (see
	// resolveTimeout), so a non-positive one reaching here is a caller that
	// skipped that, not a config file - which is why it is worth saying out
	// loud rather than quietly correcting.
	if timeoutDuration <= 0 {
		logger.GetLogger().Warn(
			"Timeout must be positive, using the default instead",
			zap.String("connector", connector),
			zap.Duration("Given", timeoutDuration),
			zap.Duration("Using", config.DefaultConnectorTimeout),
		)
		timeoutDuration = config.DefaultConnectorTimeout
	}

	sendBuffer := make(chan *message.Raw, bufferCapacity)
	defer close(sendBuffer)
	go publisher.Send(sendBuffer)

	// Reaching here is what "gosk started" means for a connect-type
	// processor, so report it to systemd now rather than leaving it to
	// the publisher's first successful send (see nanomsg/pub.go's send).
	// The publisher is already listening by the time Publish is called,
	// so everything downstream can connect, and from here on this
	// connector reports the truth about its sensor either way: data, or
	// a DisconnectedOrNoData status. Whether the sensor answers is not
	// this process's own health - that is precisely what the status
	// reports are for - and tying readiness to it meant a connector with
	// an absent sensor only became ready after a full timeoutDuration,
	// which had to be kept under gosk.nix's TimeoutStartSec (40s) for
	// the unit to start at all. That coupling is gone: a connector is
	// ready in milliseconds now, whatever its timeout is set to.
	sdnotify.Ready()

	// The timeout is selected on here rather than run as an AfterFunc, so
	// that the one goroutine which reads the stream is also the only one
	// that ever touches connected. As a callback it ran on the timer's
	// own goroutine and raced this loop for that variable, and losing
	// that race did more than tear a bool: the loop only announces
	// ConnectedAndData when it sees connected go false, so a timeout
	// firing between the loop's read of connected and its write could
	// leave the connector reporting DisconnectedOrNoData - and the
	// mapper raising an offline notification for it - every timeout for
	// as long as the process lived, while data flowed the whole time.
	//
	// Reset is safe to call on an already fired timer here: since go
	// 1.23, which go.mod is well past, it discards a value the channel
	// is still holding instead of leaving it to be received as a stale
	// timeout.
	timeout := time.NewTimer(timeoutDuration)
	defer timeout.Stop()

	// ConnectedAndData is only published on the disconnected->connected
	// transition, which for a healthy sensor happens once, in the first
	// moments of the process. nanomsg pub/sub is lossy and has no replay
	// for a late subscriber (see mapper/main.go's process), so any mapper
	// that connected after that - or restarted later, which for a mapper
	// unit is routine - never sees it, while DisconnectedOrNoData keeps
	// repeating. A mapper that had raised the offline notification and
	// then restarted would therefore leave that alarm up forever, on a
	// connector that has been fine for weeks. Re-announcing the healthy
	// state periodically makes the report eventually truthful in both
	// directions; the timeout above already covers the disconnected case,
	// so this only has to cover the other - and can be far rarer than it,
	// see connectedHeartbeatFactor.
	heartbeat := time.NewTicker(connectedHeartbeatFactor * timeoutDuration)
	defer heartbeat.Stop()

	connected := false
	for {
		select {
		case value, ok := <-stream:
			if !ok {
				return
			}
			timeout.Reset(timeoutDuration)
			if !connected {
				connected = true
				sendBuffer <- connectorStatus(connector, true)
			}
			sendBuffer <- message.NewRaw().WithConnector(connector).WithValue(value).WithType(protocol)
		case <-timeout.C:
			if connected {
				logger.GetLogger().Warn(
					"timeout receiving data for the stream, no data received",
					zap.String("connector", connector),
				)
			}
			connected = false
			sendBuffer <- connectorStatus(connector, false)
			timeout.Reset(timeoutDuration)
		case <-heartbeat.C:
			if connected {
				sendBuffer <- connectorStatus(connector, true)
			}
		}
	}
}

func connectorStatus(connector string, connected bool) *message.Raw {
	value := message.ConnectorStatusDisconnectedOrNoData
	if connected {
		value = message.ConnectorStatusConnectedAndData
	}
	return message.NewRaw().WithConnector(connector).WithType(message.ConnectorStatusType).WithValue([]byte(value))
}
