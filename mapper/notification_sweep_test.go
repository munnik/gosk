package mapper

import (
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
)

// TestNotificationMapperSweep verifies that refreshMap, the periodic
// re-evaluation driven by process's ticker (see periodicMapper in main.go),
// notices a check whose source paths have all gone completely silent even
// though nothing arrives to trigger DoMap for it.
func TestNotificationMapperSweep(t *testing.T) {
	checks := []*config.NotificationMappingConfig{
		{
			MappingConfig: config.MappingConfig{
				Path:       "notifications.test.sweep",
				Expression: "test_sweepPrimary.Value > 100",
			},
			SourcePaths: []string{"test.sweepPrimary", "test.sweepSecondary"},
			Message:     "sweep test message",
			State:       "alarm",
			Timeout:     30 * time.Second,
		},
	}
	m, err := NewNotificationMapper(config.MapperConfig{Context: "testingContext"}, checks)
	if err != nil {
		t.Fatalf("NewNotificationMapper: %v", err)
	}

	now := time.Now()
	input := message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
		AddUpdate(
			message.NewUpdate().WithTimestamp(now).AddValue(
				message.NewValue().WithPath("test.sweepPrimary").WithValue(50.0),
			),
		).
		AddUpdate(
			message.NewUpdate().WithTimestamp(now).AddValue(
				message.NewValue().WithPath("test.sweepSecondary").WithValue(1.0),
			),
		)
	if _, err := m.DoMap(input); err != nil {
		t.Fatalf("DoMap: %v", err)
	}

	if out := m.refreshMap(now.Add(10 * time.Second)); len(out.Updates) != 0 {
		t.Fatalf("expected no updates before Timeout elapses, got %+v", out.Updates)
	}

	out := m.refreshMap(now.Add(40 * time.Second))
	if len(out.Updates) != 1 {
		t.Fatalf("expected exactly one update once Timeout elapses with no new data, got %d: %+v", len(out.Updates), out.Updates)
	}
	values := out.Updates[0].Values
	if len(values) != 1 || values[0].Path != "notifications.test.sweep" {
		t.Fatalf("unexpected update values: %+v", values)
	}
	notification, ok := values[0].Value.(message.Notification)
	if !ok {
		t.Fatalf("expected a message.Notification, got %T: %+v", values[0].Value, values[0].Value)
	}
	if notification.State == nil || *notification.State != "alarm" {
		t.Fatalf("expected state \"alarm\", got %+v", notification.State)
	}
	if notification.Message == nil || *notification.Message != "sweep test message" {
		t.Fatalf("expected the configured message, got %+v", notification.Message)
	}

	// once confirmed raised, sweeping again at the same stale state should
	// not repeat the notification
	if out := m.refreshMap(now.Add(41 * time.Second)); len(out.Updates) != 0 {
		t.Fatalf("expected no repeated update for an unchanged confirmed state, got %+v", out.Updates)
	}
}
