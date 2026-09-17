package sdnotify_test

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/munnik/gosk/sdnotify"
)

func TestReadyIsANoOpWithoutNotifySocket(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	// must not panic or block
	sdnotify.Ready()
}

func TestReadyIgnoresAnUndialableSocket(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", filepath.Join(t.TempDir(), "does-not-exist.sock"))
	// must not panic or block, just silently fail to notify
	sdnotify.Ready()
}

func TestReadySendsReadyOnTheNotifySocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "notify.sock")
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		t.Fatalf("could not listen on a test unixgram socket: %v", err)
	}
	defer listener.Close()

	t.Setenv("NOTIFY_SOCKET", socketPath)
	sdnotify.Ready()

	listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := listener.Read(buf)
	if err != nil {
		t.Fatalf("expected a READY datagram, got error: %v", err)
	}
	if got := string(buf[:n]); got != "READY=1" {
		t.Fatalf("expected %q, got %q", "READY=1", got)
	}
}

func TestExtendTimeoutIsANoOpWithoutNotifySocket(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	// must not panic or block
	sdnotify.ExtendTimeout(time.Second)
}

func TestExtendTimeoutSendsExtendTimeoutUsecOnTheNotifySocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "notify.sock")
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		t.Fatalf("could not listen on a test unixgram socket: %v", err)
	}
	defer listener.Close()

	t.Setenv("NOTIFY_SOCKET", socketPath)
	// clear any throttle window a previous test left behind
	time.Sleep(sdnotify.ExtendTimeoutMinInterval)
	sdnotify.ExtendTimeout(10 * time.Second)

	listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := listener.Read(buf)
	if err != nil {
		t.Fatalf("expected an EXTEND_TIMEOUT_USEC datagram, got error: %v", err)
	}
	if got, want := string(buf[:n]), "EXTEND_TIMEOUT_USEC=10000000"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestExtendTimeoutIsThrottled(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "notify.sock")
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		t.Fatalf("could not listen on a test unixgram socket: %v", err)
	}
	defer listener.Close()

	t.Setenv("NOTIFY_SOCKET", socketPath)
	time.Sleep(sdnotify.ExtendTimeoutMinInterval)

	sdnotify.ExtendTimeout(time.Second)
	listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	if _, err := listener.Read(buf); err != nil {
		t.Fatalf("expected the first call to send a datagram, got error: %v", err)
	}

	sdnotify.ExtendTimeout(time.Second) // within the throttle window - must be suppressed
	listener.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	if _, err := listener.Read(buf); err == nil {
		t.Fatalf("expected the throttled call to be suppressed, but a second datagram arrived")
	}
}

func TestReadyHandlesAnAbstractSocketAddress(t *testing.T) {
	// abstract sockets aren't backed by a filesystem path, so a fixed,
	// sufficiently unique name avoids colliding with another process -
	// the leading "@" (mapped to a NUL byte on the wire) is what makes it
	// abstract in the first place.
	name := "gosk-sdnotify-test-abstract"
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: "@" + name, Net: "unixgram"})
	if err != nil {
		t.Skipf("abstract unix sockets not supported on this platform: %v", err)
	}
	defer listener.Close()

	t.Setenv("NOTIFY_SOCKET", "@"+name)
	sdnotify.Ready()

	listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := listener.Read(buf)
	if err != nil {
		t.Fatalf("expected a READY datagram, got error: %v", err)
	}
	if got := string(buf[:n]); got != "READY=1" {
		t.Fatalf("expected %q, got %q", "READY=1", got)
	}
}
