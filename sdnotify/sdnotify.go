// Package sdnotify sends systemd's sd_notify readiness datagram, the
// minimal wire protocol only (no cgo, no dependency on
// github.com/coreos/go-systemd): write "READY=1" to the unix datagram
// socket named by $NOTIFY_SOCKET.
package sdnotify

import (
	"net"
	"os"
	"strings"

	"github.com/munnik/gosk/logger"
	"go.uber.org/zap"
)

// Ready tells systemd (for a Type=notify unit) that this process is done
// starting. A silent no-op if $NOTIFY_SOCKET isn't set - running outside
// systemd, or the unit isn't Type=notify - or if the write fails:
// readiness notification only affects how the fleet's gosk-*.service
// units are ordered against each other at startup (see the nix module
// wiring this up), never something worth this process failing over.
func Ready() {
	addr := os.Getenv("NOTIFY_SOCKET")
	if addr == "" {
		return
	}
	// systemd also accepts an abstract Linux socket, spelled with a
	// leading "@" in $NOTIFY_SOCKET but an actual leading NUL byte on the
	// wire.
	if strings.HasPrefix(addr, "@") {
		addr = "\x00" + addr[1:]
	}
	conn, err := net.Dial("unixgram", addr)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not dial NOTIFY_SOCKET",
			zap.String("Error", err.Error()),
		)
		return
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("READY=1")); err != nil {
		logger.GetLogger().Warn(
			"Could not notify systemd of readiness",
			zap.String("Error", err.Error()),
		)
	}
}
