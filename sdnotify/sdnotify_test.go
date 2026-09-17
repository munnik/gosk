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
