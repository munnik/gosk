package mapper

import (
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
)

func mappedOf(values ...*message.Value) *message.Mapped {
	u := message.NewUpdate().WithSource(*message.NewSource().WithLabel("connectAmpero").WithType("modbus")).WithTimestamp(time.Now())
	for _, v := range values {
		u.AddValue(v)
	}
	return message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(u)
}

func diagnosticValuesByPath(m *message.Mapped) map[string]message.Value {
	result := make(map[string]message.Value)
	for _, u := range m.Updates {
		for _, v := range u.Values {
			result[v.Path] = v
		}
	}
	return result
}

// A filter cannot judge a reading that has no value; trying used to fail the
// expression and drop the whole message.
func TestExpressionFilterPassesARejectedReading(t *testing.T) {
	f, err := NewExpressionFilter([]*config.ExpressionMappingConfig{{
		MappingConfig: config.MappingConfig{Path: "tanks.fuel.aft.currentLevel", Expression: "tanks_fuel_aft_currentLevel.Value < 0"},
		SourcePaths:   []string{"tanks.fuel.aft.currentLevel"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.DoMap(mappedOf(
		message.NewValue().WithPath("tanks.fuel.aft.currentLevel").WithValue(nil).WithDiagnostics(map[string]any{message.DiagnosticRejected: "all zeros"}),
		message.NewValue().WithPath("navigation.speedOverGround").WithValue(3.0),
	))
	if err != nil {
		t.Fatalf("a rejected reading failed the filter: %v", err)
	}
	got := diagnosticValuesByPath(out)
	if len(got) != 2 || !message.IsRejected(got["tanks.fuel.aft.currentLevel"].Diagnostics) || got["navigation.speedOverGround"].Value != 3.0 {
		t.Fatalf("got %+v", got)
	}

	// and the filter still filters a reading that has a value
	out, err = f.DoMap(mappedOf(message.NewValue().WithPath("tanks.fuel.aft.currentLevel").WithValue(-1.0)))
	if err != nil || len(diagnosticValuesByPath(out)) != 0 {
		t.Fatalf("a negative level passed the filter: %+v, %v", out, err)
	}
}

func TestAggregateTransformKeepsTheReadingsDiagnostics(t *testing.T) {
	transform := &config.ExpressionMappingConfig{
		MappingConfig: config.MappingConfig{Path: "a.level", Expression: "a_level.Value == nil ? nil : a_level.Value * 2"},
		SourcePaths:   []string{"a.level"},
		Overwrite:     true,
	}
	reader := &config.ExpressionMappingConfig{
		MappingConfig: config.MappingConfig{Path: "a.current", Expression: "a_level.Diagnostics.current"},
		SourcePaths:   []string{"a.level"},
	}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, []*config.ExpressionMappingConfig{transform, reader})
	if err != nil {
		t.Fatal(err)
	}

	out, err := m.DoMap(mappedOf(message.NewValue().WithPath("a.level").WithValue(2.0).WithDiagnostics(map[string]any{"current": 0.012})))
	if err != nil {
		t.Fatal(err)
	}
	got := diagnosticValuesByPath(out)
	if got["a.level"].Value != 4.0 || got["a.level"].Diagnostics["current"] != 0.012 {
		t.Fatalf("the corrected value lost its diagnostics: %+v", got["a.level"])
	}
	if got["a.current"].Value != 0.012 {
		t.Fatalf("a mapping reading the corrected path did not see its diagnostics: %+v", got)
	}
	if got["a.current"].Diagnostics != nil {
		t.Fatalf("a derived value inherited diagnostics: %+v", got["a.current"])
	}

	out, err = m.DoMap(mappedOf(message.NewValue().WithPath("a.level").WithValue(nil).WithDiagnostics(map[string]any{message.DiagnosticRejected: "all zeros"})))
	if err != nil {
		t.Fatal(err)
	}
	if v := diagnosticValuesByPath(out)["a.level"]; v.Value != nil || !message.IsRejected(v.Diagnostics) {
		t.Fatalf("a transform stripped the rejection: %+v", v)
	}
}

// A rejected reading is not a sample: in history it would fail the numeric
// functions over it until it aged out, long after the sensor recovered.
func TestAggregateKeepsRejectedReadingsOutOfHistory(t *testing.T) {
	average := &config.ExpressionMappingConfig{
		MappingConfig: config.MappingConfig{Path: "a.level.average", Expression: "movingAverage(history['a_level'])"},
		SourcePaths:   []string{"a.level"},
		RetentionTime: time.Minute,
	}
	why := &config.ExpressionMappingConfig{
		MappingConfig: config.MappingConfig{Path: "a.level.why", Expression: "a_level.Diagnostics.rejected"},
		SourcePaths:   []string{"a.level"},
	}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, []*config.ExpressionMappingConfig{average, why})
	if err != nil {
		t.Fatal(err)
	}
	reading := func(v any, d map[string]any) map[string]message.Value {
		t.Helper()
		out, err := m.DoMap(mappedOf(message.NewValue().WithPath("a.level").WithValue(v).WithDiagnostics(d)))
		if err != nil {
			t.Fatal(err)
		}
		return diagnosticValuesByPath(out)
	}
	rejected := map[string]any{message.DiagnosticRejected: "all zeros"}

	// all rejected: nothing to average, and the expressions reading the
	// path still see why
	if got := reading(nil, rejected); got["a.level.why"].Value != "all zeros" || got["a.level.average"].Value != nil {
		t.Fatalf("all rejected: %+v", got)
	}

	if got := reading(2.0, nil); got["a.level.average"].Value != 2.0 {
		t.Fatalf("first reading: %+v", got)
	}
	reading(nil, rejected)
	if got := reading(4.0, nil); got["a.level.average"].Value != 3.0 {
		t.Fatalf("valid, rejected, valid within one window averaged %+v, want 3", got["a.level.average"])
	}
	if h := m.env["history"].(map[string][]message.SingleValueMapped)["a_level"]; len(h) != 2 {
		t.Fatalf("history holds %d entries, want the two readings", len(h))
	}
}
