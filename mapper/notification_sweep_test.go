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
// though nothing arrives to trigger DoMap for it, and that it republishes a
// check's confirmed state - notifying or cleared - on every sweep so a lost
// announcement self-heals within one tick interval either way.
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

	// before Timeout elapses, sweeping republishes the confirmed-cleared
	// state (the same self-heal a notifying check gets, see below) but does
	// not flip to alarm from staleness yet
	out := m.refreshMap(now.Add(10 * time.Second))
	if len(out.Updates) != 1 {
		t.Fatalf("expected the confirmed-cleared state to be republished, got %d: %+v", len(out.Updates), out.Updates)
	}
	if out.Updates[0].Values[0].Value != nil {
		t.Fatalf("expected the republished value to still be cleared (nil), got %+v", out.Updates[0].Values[0].Value)
	}

	out = m.refreshMap(now.Add(40 * time.Second))
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

	// once confirmed raised, sweeping again at the same stale state
	// re-publishes it: this is what lets a lost or missed announcement (e.g.
	// a subscriber that has not finished connecting yet when the check was
	// first confirmed) self-heal within one tick interval, instead of the
	// notifying state staying silently correct in memory forever
	out = m.refreshMap(now.Add(41 * time.Second))
	if len(out.Updates) != 1 {
		t.Fatalf("expected the still-notifying confirmed state to be republished, got %d: %+v", len(out.Updates), out.Updates)
	}
	notification, ok = out.Updates[0].Values[0].Value.(message.Notification)
	if !ok || notification.State == nil || *notification.State != "alarm" {
		t.Fatalf("expected the republished value to still be the alarm notification, got %+v", out.Updates[0].Values[0].Value)
	}

	// the check clears once fresh data brings the primary value back down -
	// refresh both source paths, or the now-stale secondary would trip the
	// Timeout fail-safe and keep the check confirmed notifying
	clearedAt := now.Add(42 * time.Second)
	cleared := message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
		AddUpdate(
			message.NewUpdate().WithTimestamp(clearedAt).AddValue(
				message.NewValue().WithPath("test.sweepPrimary").WithValue(50.0),
			),
		).
		AddUpdate(
			message.NewUpdate().WithTimestamp(clearedAt).AddValue(
				message.NewValue().WithPath("test.sweepSecondary").WithValue(1.0),
			),
		)
	if _, err := m.DoMap(cleared); err != nil {
		t.Fatalf("DoMap: %v", err)
	}

	// sweeping again at the same now-cleared state re-publishes the clear
	// too: a downstream sink that persists the latest value per path (e.g.
	// the database writer) keeps serving a stale "alarm" forever if the
	// one-shot clear is ever lost (e.g. a subscriber that has not finished
	// connecting yet right as the check clears), so cleared checks self-heal
	// on sweep exactly like notifying ones do
	out = m.refreshMap(clearedAt.Add(1 * time.Second))
	if len(out.Updates) != 1 {
		t.Fatalf("expected the confirmed-cleared state to be republished, got %d: %+v", len(out.Updates), out.Updates)
	}
	values = out.Updates[0].Values
	if len(values) != 1 || values[0].Path != "notifications.test.sweep" {
		t.Fatalf("unexpected update values: %+v", values)
	}
	if values[0].Value != nil {
		t.Fatalf("expected the republished value to still be cleared (nil), got %+v", values[0].Value)
	}
}
