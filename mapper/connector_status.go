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
	u := message.NewUpdate().
		WithSource(*message.NewSource().WithLabel("notification").WithType(config.SignalKType)).
		WithTimestamp(time.Now())

	path := connectorStatusPath(connector)
	if connected {
		u.AddValue(message.NewValue().WithPath(path).WithValue(nil))
	} else {
		state := "alarm"
		msg := fmt.Sprintf("no data received from %s", connector)
		u.AddValue(message.NewValue().WithPath(path).WithValue(message.Notification{State: &state, Message: &msg}))
	}

	return message.NewMapped().WithContext(context).WithOrigin(context).AddUpdate(u)
}
