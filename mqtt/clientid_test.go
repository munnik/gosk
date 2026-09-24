package mqtt

import (
	"os"
	"strings"
	"testing"

	"github.com/munnik/gosk/config"
)

func TestClientIDPrefersTheConfiguredValue(t *testing.T) {
	c := &config.MQTTConfig{ClientID: "gosk-something-specific"}
	if got := clientID(c, "write"); got != "gosk-something-specific" {
		t.Errorf("got %q, want the configured client id", got)
	}
}

func TestClientIDIsDerivedFromRoleAndHost(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Skipf("no hostname available: %v", err)
	}

	got := clientID(&config.MQTTConfig{}, "write")

	if !strings.HasPrefix(got, "gosk-write-") {
		t.Errorf("got %q, want it to name the role", got)
	}
	if !strings.Contains(got, sanitizeClientID(host)) {
		t.Errorf("got %q, want it to contain the hostname %q", got, host)
	}

	// Two roles on one host have to differ, otherwise the two processes
	// take turns disconnecting each other.
	if other := clientID(&config.MQTTConfig{}, "read"); other == got {
		t.Errorf("the write and read roles both derived %q", got)
	}
}

func TestClientIDIsStableAcrossCalls(t *testing.T) {
	// A client id that changed per process start would give up the
	// persistent session, which is the whole point of setting one.
	first := clientID(&config.MQTTConfig{}, "transferRespond")
	if second := clientID(&config.MQTTConfig{}, "transferRespond"); first != second {
		t.Errorf("derived %q and then %q for the same role", first, second)
	}
}

func TestSanitizeClientIDReplacesAwkwardCharacters(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"gosk-write-vessel01", "gosk-write-vessel01"},
		{"gosk-write-vessel.example.com", "gosk-write-vessel-example-com"},
		{"gosk-write-a b/c", "gosk-write-a-b-c"},
		{"gosk_write_1", "gosk_write_1"},
	} {
		if got := sanitizeClientID(tc.in); got != tc.want {
			t.Errorf("sanitizeClientID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
