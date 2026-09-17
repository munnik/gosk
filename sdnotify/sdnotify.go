// Package sdnotify sends systemd's sd_notify readiness notification via
// github.com/coreos/go-systemd/v22/daemon.
package sdnotify

import (
	"fmt"
	"sync"
	"time"

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

// ExtendTimeoutMinInterval throttles ExtendTimeout to at most one actual
// notify-socket write per interval, regardless of how often it's called -
// a caller like the FFT mapper (see mapper/fft.go) may be driven by a
// per-sample loop running at kHz rates, and there's no need to write to
// the socket anywhere near that often just to prove the process is alive.
var ExtendTimeoutMinInterval = 5 * time.Second

var (
	lastExtend   time.Time
	lastExtendMu sync.Mutex
)

// ExtendTimeout tells systemd (for a Type=notify unit) to push its
// start/stop/runtime timeout out to roughly d from now, for a process
// that is doing real work but genuinely isn't ready yet - e.g. the FFT
// mapper, which must buffer several seconds of samples before it can
// publish its first spectrum and call Ready(). Call it repeatedly for as
// long as that's true; it's throttled internally (see
// ExtendTimeoutMinInterval) so calling it on every input is fine. Once
// Ready() has been called there's nothing left to extend, so callers
// should stop calling this. Like Ready(), a silent no-op without
// $NOTIFY_SOCKET or on failure - this only affects fleet startup
// ordering/timeouts, never worth failing over.
func ExtendTimeout(d time.Duration) {
	lastExtendMu.Lock()
	if !lastExtend.IsZero() && time.Since(lastExtend) < ExtendTimeoutMinInterval {
		lastExtendMu.Unlock()
		return
	}
	lastExtend = time.Now()
	lastExtendMu.Unlock()

	state := fmt.Sprintf("EXTEND_TIMEOUT_USEC=%d", d.Microseconds())
	if _, err := daemon.SdNotify(false, state); err != nil {
		logger.GetLogger().Warn(
			"Could not extend systemd's start timeout",
			zap.String("Error", err.Error()),
		)
	}
}
