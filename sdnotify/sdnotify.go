// Package sdnotify sends systemd's sd_notify readiness notification via
// github.com/coreos/go-systemd/v22/daemon.
package sdnotify

import (
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

// Ready tells systemd (for a Type=notify unit) that this process is done
// starting. A silent no-op if $NOTIFY_SOCKET isn't set - running outside
// systemd, or the unit isn't Type=notify - or if the notification fails:
// readiness notification only affects how the fleet's gosk-*.service
// units are ordered against each other at startup (see the nix module
// wiring this up), never something worth this process failing over.
func Ready() {
	if _, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		logger.GetLogger().Warn(
			"Could not notify systemd of readiness",
			zap.String("Error", err.Error()),
		)
	}
}
