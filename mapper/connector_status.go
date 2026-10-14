package mapper

import (
	"fmt"
	"strings"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
)

// ConnectorStatusMapper is implemented by a mapper that can turn a
// connector's own online/offline report (see message.ConnectorStatusType,
// published by connector/main.go's process instead of the process exiting
// when its sensor never connects or goes quiet) into a SignalK
// notification. process (see main.go) checks for this before handing such
// a report to the mapper's usual DoMap, since DoMap only knows how to
// decode real protocol data. A mapper that doesn't implement this
// interface silently drops connector status reports rather than passing
// them to DoMap, which would otherwise misinterpret them as malformed
// protocol data.
type ConnectorStatusMapper interface {
	MapConnectorStatus(connector string, connected bool) *message.Mapped
}

// connectorStatusPath is the SignalK notification path a connector's own
// status is published under - see NewConnectorStatusUpdate.
func connectorStatusPath(connector string) string {
	return "notifications.connectors." + strings.ReplaceAll(connector, " ", "") + ".connected"
}

// NewConnectorStatusUpdate builds the SignalK notification a
// ConnectorStatusMapper implementation publishes for a connector's own
// online/offline report. context is the mapper's own vessel context (e.g.
// config.MapperConfig.Context), the same one it stamps on everything else
// it maps.
//
// connected clears any previously raised notification (mirrors
// NotificationMapper.evaluateCheck's own not-notifying case: Value nil),
// so a connector that recovers is exactly as visible as one that goes
// down. !connected raises an alarm - unlike NotificationMapper's
// hysteresis-gated checks, this has no set/reset delay: the connector
// layer's own timeout (see connector/main.go's process) is already the
// debounce, there is no benefit to a second one on top of it.
func NewConnectorStatusUpdate(context, connector string, connected bool) *message.Mapped {
	return newSourceStatusUpdate(context, connectorStatusPath(connector), connector, connected)
}

// readerStatusPath is the SignalK notification path a reader's own status is
// published under - see NewReaderStatusUpdate. A separate namespace from
// connectorStatusPath on purpose: a reader's source is a broker carrying a
// whole fleet's data, not a sensor on one vessel, so "the reader's source is
// gone" and "a vessel's sensor is gone" want to be told apart by whatever is
// alarming on them.
func readerStatusPath(reader string) string {
	return "notifications.readers." + strings.ReplaceAll(reader, " ", "") + ".connected"
}

// NewReaderStatusUpdate is NewConnectorStatusUpdate for a reader (see
// reader/mqtt.go), whose source going quiet means the broker is unreachable
// or the fleet has stopped publishing.
//
// A reader publishes this itself rather than handing a Raw status report to
// a mapper the way a connector does, because a reader publishes
// message.Mapped already - there is no mapping stage downstream of it to do
// the converting. context is config.ReaderConfig.Context, since a reader
// aggregates many vessels and has no vessel context of its own.
func NewReaderStatusUpdate(context, reader string, connected bool) *message.Mapped {
	return newSourceStatusUpdate(context, readerStatusPath(reader), reader, connected)
}

// newSourceStatusUpdate is the notification both of the above publish, so
// that a connector's and a reader's reports stay the same shape - only the
// path differs. name is what the alarm message names as the silent source.
func newSourceStatusUpdate(context, path, name string, connected bool) *message.Mapped {
	u := message.NewUpdate().
		WithSource(*message.NewSource().WithLabel("notification").WithType(config.SignalKType)).
		WithTimestamp(time.Now())

	if connected {
		u.AddValue(message.NewValue().WithPath(path).WithValue(nil))
	} else {
		state := "alarm"
		msg := fmt.Sprintf("no data received from %s", name)
		u.AddValue(message.NewValue().WithPath(path).WithValue(message.Notification{State: &state, Message: &msg}))
	}

	return message.NewMapped().WithContext(context).WithOrigin(context).AddUpdate(u)
}
