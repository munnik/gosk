package mapper

import (
	"testing"
	"time"

	"github.com/munnik/gosk/config"
)

func TestPlausibilityGuardWaitsForInput(t *testing.T) {
	n := &config.NotificationMappingConfig{
		MappingConfig: config.MappingConfig{Path: "notifications.sensor.overload", Expression: "sensor.Value > 1.1"},
		When:          "sensor != nil", SourcePaths: []string{"sensor"}, SetDelay: 2 * time.Minute, ResetDelay: 30 * time.Second,
	}
	m, err := NewNotificationMapper(config.MapperConfig{Context: "testingContext"}, []*config.NotificationMappingConfig{n})
	if err != nil {
		t.Fatal(err)
	}
	m.refreshMap(time.Now())
	if m.states[n].confirmedNotifying {
		t.Fatal("missing input raised a plausibility warning")
	}
	now := time.Now()
	m.DoMap(multiValueUpdate(now, "sensor", 2.0))
	if m.states[n].confirmedNotifying {
		t.Fatal("set delay was bypassed")
	}
	m.refreshMap(now.Add(2 * time.Minute))
	if !m.states[n].confirmedNotifying {
		t.Fatal("available out-of-range input did not warn")
	}
	m.DoMap(multiValueUpdate(now.Add(3*time.Minute), "sensor", 1.0))
	m.refreshMap(now.Add(4 * time.Minute))
	if m.states[n].confirmedNotifying {
		t.Fatal("normal value did not clear warning")
	}
}
