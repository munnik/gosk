package connector

import (
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

const bufferCapacity = 5000

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
// long as the stream stays quiet, rather than firing once: config.Timeout
// now defaults to 30s specifically so this keeps systemd satisfied
// (gosk.nix's TimeoutStartSec is 40s, comfortably longer) for as long as
// the sensor is genuinely absent, not just for the first 30s of it.
func process(stream <-chan []byte, connector string, protocol string, publisher *nanomsg.Publisher[message.Raw], timeoutDuration time.Duration) {
	sendBuffer := make(chan *message.Raw, bufferCapacity)
	defer close(sendBuffer)
	go publisher.Send(sendBuffer)

	connected := false
	var timeout *time.Timer
	markDisconnected := func() {
		if connected {
			logger.GetLogger().Warn(
				"timeout receiving data for the stream, no data received",
				zap.String("connector", connector),
			)
		}
		connected = false
		sendBuffer <- connectorStatus(connector, false)
		timeout.Reset(timeoutDuration)
	}
	timeout = time.AfterFunc(timeoutDuration, markDisconnected)
	defer timeout.Stop()

	var m *message.Raw
	for value := range stream {
		timeout.Reset(timeoutDuration)
		if !connected {
			connected = true
			sendBuffer <- connectorStatus(connector, true)
		}
		m = message.NewRaw().WithConnector(connector).WithValue(value).WithType(protocol)
		sendBuffer <- m
	}
}

func connectorStatus(connector string, connected bool) *message.Raw {
	value := message.ConnectorStatusDisconnectedOrNoData
	if connected {
		value = message.ConnectorStatusConnectedAndData
	}
	return message.NewRaw().WithConnector(connector).WithType(message.ConnectorStatusType).WithValue([]byte(value))
}
