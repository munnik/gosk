package connector

import (
	"net/url"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/protocol"
)

// TestDefaultTimeoutFollowsThePollingInterval covers the default a connector
// gets when its config file sets no timeout of its own.
//
// A flat default cannot suit both a connector polling twice a second and one
// reading tank levels once a minute: 30s is far too long to notice the first
// has died, and far too short for the second, which would raise an alarm in
// every gap between its polls and clear it on each one. The config already
// says how slowly the connector polls, so the default is read off that.
func TestDefaultTimeoutFollowsThePollingInterval(t *testing.T) {
	for _, test := range []struct {
		name      string
		intervals []time.Duration
		want      time.Duration
	}{
		{
			// The case this exists for: tank levels read once a minute get
			// the two minutes an operator would otherwise have to write out.
			name:      "a tank level read once a minute",
			intervals: []time.Duration{time.Minute},
			want:      2 * time.Minute,
		},
		{
			// Twice the slowest, not the fastest: the fast group going quiet
			// leaves the slow one as the only thing still arriving, and a
			// timeout shorter than its interval would flap between its polls.
			name:      "a fast group alongside a slow one",
			intervals: []time.Duration{500 * time.Millisecond, time.Minute},
			want:      2 * time.Minute,
		},
		{
			// Doubling a brisk interval gives something far too twitchy for
			// one late response to be an alarm, so the floor applies.
			name:      "only fast groups",
			intervals: []time.Duration{500 * time.Millisecond, time.Second},
			want:      config.DefaultConnectorTimeout,
		},
		{
			name:      "a connector that polls nothing",
			intervals: nil,
			want:      config.DefaultConnectorTimeout,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := config.DefaultTimeoutFor(test.intervals...); got != test.want {
				t.Errorf("DefaultTimeoutFor(%v) = %v, want %v", test.intervals, got, test.want)
			}
		})
	}
}

// TestConfiguredTimeoutWins is the other half: a connector that does say what
// its timeout should be gets exactly that, however it polls. This is what the
// readiness work made safe to set freely - it used to have to stay under
// gosk.nix's TimeoutStartSec (40s) or the unit would never start.
func TestConfiguredTimeoutWins(t *testing.T) {
	u, err := url.Parse("tcp://127.0.0.1:5020")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	c := &config.ConnectorConfig{
		Name: "tank levels", URL: u, Protocol: config.ModbusType,
		DataBits: 8, StopBits: "1", Parity: "N",
		Timeout: 2 * time.Minute,
	}
	// A polling interval that would derive something far shorter, to prove
	// the configured value is not merely coinciding with a derived one.
	conn, err := NewModbusConnector(c, []config.RegisterGroupConfig{{
		ModbusHeader: protocol.ModbusHeader{
			Slave: 1, FunctionCode: protocol.ReadInputRegisters,
			Address: 1043, NumberOfCoilsOrRegisters: 1,
		},
		PollingInterval: time.Second,
	}})
	if err != nil {
		t.Fatalf("NewModbusConnector: %v", err)
	}
	if conn.timeout != 2*time.Minute {
		t.Errorf("got timeout %v, want the configured 2m", conn.timeout)
	}
	if c.Timeout != 2*time.Minute {
		t.Errorf("the connector changed the config it was handed: %v", c.Timeout)
	}
}

// TestModbusConnectorDerivesItsTimeout checks the derivation is actually
// wired into the connector, not just available to it.
func TestModbusConnectorDerivesItsTimeout(t *testing.T) {
	u, err := url.Parse("tcp://127.0.0.1:5020")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	c := &config.ConnectorConfig{
		Name: "tank levels", URL: u, Protocol: config.ModbusType,
		DataBits: 8, StopBits: "1", Parity: "N",
		// Unset, as config.NewConnectorConfig leaves it.
	}
	conn, err := NewModbusConnector(c, []config.RegisterGroupConfig{{
		ModbusHeader: protocol.ModbusHeader{
			Slave: 1, FunctionCode: protocol.ReadInputRegisters,
			Address: 1043, NumberOfCoilsOrRegisters: 1,
		},
		PollingInterval: time.Minute,
	}})
	if err != nil {
		t.Fatalf("NewModbusConnector: %v", err)
	}
	if conn.timeout != 2*time.Minute {
		t.Errorf("got timeout %v, want 2m derived from the 1m polling interval", conn.timeout)
	}
}

// TestHttpConnectorDerivesItsTimeout is TestModbusConnectorDerivesItsTimeout
// for the other connector that polls on configured intervals.
func TestHttpConnectorDerivesItsTimeout(t *testing.T) {
	u, err := url.Parse("tcp://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	conn, err := NewHttpConnector(
		&config.ConnectorConfig{Name: "weather", URL: u, Protocol: config.HttpType},
		[]config.UrlGroupConfig{
			{Url: "http://example.invalid/fast", PollingInterval: time.Second},
			{Url: "http://example.invalid/slow", PollingInterval: 5 * time.Minute},
		},
	)
	if err != nil {
		t.Fatalf("NewHttpConnector: %v", err)
	}
	if conn.timeout != 10*time.Minute {
		t.Errorf("got timeout %v, want 10m derived from the slowest (5m) url group", conn.timeout)
	}
}

// TestStreamConnectorsUseTheFlatDefault covers the connectors that read a
// stream rather than polling: there is no interval in their config to read a
// default off, so they land on config.DefaultConnectorTimeout.
func TestStreamConnectorsUseTheFlatDefault(t *testing.T) {
	u, err := url.Parse("tcp://127.0.0.1:13400")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	c := &config.ConnectorConfig{
		Name: "NMEA0183", URL: u, Protocol: "nmea0183",
		BaudRate: 4800, DataBits: 8, StopBits: "1", Parity: "N",
	}

	line, err := NewLineConnector(c)
	if err != nil {
		t.Fatalf("NewLineConnector: %v", err)
	}
	if line.timeout != config.DefaultConnectorTimeout {
		t.Errorf("line: got timeout %v, want %v", line.timeout, config.DefaultConnectorTimeout)
	}

	canbus, err := NewCanBusConnector(c)
	if err != nil {
		t.Fatalf("NewCanBusConnector: %v", err)
	}
	if canbus.timeout != config.DefaultConnectorTimeout {
		t.Errorf("canbus: got timeout %v, want %v", canbus.timeout, config.DefaultConnectorTimeout)
	}

	manner, err := NewMannerEthernetConnector(c)
	if err != nil {
		t.Fatalf("NewMannerEthernetConnector: %v", err)
	}
	if manner.timeout != config.DefaultConnectorTimeout {
		t.Errorf("manner ethernet: got timeout %v, want %v", manner.timeout, config.DefaultConnectorTimeout)
	}
}
