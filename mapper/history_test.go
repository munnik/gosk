package mapper_test

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
)

// series builds a history window: one sample per step seconds, oldest
// first, the way an aggregate's history['path'] is stored.
func series(step float64, values ...any) []message.SingleValueMapped {
	t0 := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	result := make([]message.SingleValueMapped, 0, len(values))
	for i, v := range values {
		result = append(result, message.SingleValueMapped{
			Path:      "test",
			Timestamp: t0.Add(time.Duration(float64(i) * step * float64(time.Second))),
			Value:     v,
		})
	}
	return result
}

func TestDeltaAndRateOfChange(t *testing.T) {
	if d := Delta(series(1)); d != 0 {
		t.Errorf("empty window: delta %v, want 0", d)
	}
	if d := Delta(series(1, 4.5)); d != 0 {
		t.Errorf("one sample: delta %v, want 0", d)
	}
	w := series(60, 10.0, 9.8, 9.5, 9.1)
	if d := Delta(w); math.Abs(d-(-0.9)) > 1e-9 {
		t.Errorf("delta %v, want -0.9", d)
	}
	if r := RateOfChange(w); math.Abs(r-(-0.9/180)) > 1e-12 {
		t.Errorf("rate %v, want %v", r, -0.9/180)
	}
	// Two samples with the same timestamp cannot have a rate.
	same := series(0, 1.0, 2.0)
	if r := RateOfChange(same); r != 0 {
		t.Errorf("same timestamps: rate %v, want 0", r)
	}
	if d := Delta(same); d != 1 {
		t.Errorf("same timestamps: delta %v, want 1", d)
	}
}

func TestDeltaSkipsNonNumericAndTakesInts(t *testing.T) {
	w := series(1, 2.0, "n/a", nil, 5, 7.5)
	if d := Delta(w); d != 5.5 {
		t.Errorf("delta %v, want 5.5 (ints counted, junk skipped)", d)
	}
}

func TestKalmanTooFewSamples(t *testing.T) {
	k := Kalman(series(1), 1e-6, 0.01)
	if k.Surprise != 0 || k.Samples != 0 || math.Abs(k.Sigma-0.1) > 1e-12 {
		t.Errorf("empty window: %+v", k)
	}
	k = Kalman(series(1, 3.0), 1e-6, 0.01)
	if k.Surprise != 0 || k.Estimate != 3 || k.Prediction != 3 || k.Samples != 1 {
		t.Errorf("one sample: %+v", k)
	}
}

func TestKalmanFollowsASteadyDrain(t *testing.T) {
	// A tank draining at 0.01 m³ per minute, read every 5 s with a little
	// jitter: the filter should track it and not be surprised by any sample.
	values := make([]any, 0, 120)
	for i := range 120 {
		x := 20.0 - 0.01*float64(i)*5/60
		if i%3 == 0 {
			x += 0.004
		} else if i%3 == 1 {
			x -= 0.004
		}
		values = append(values, x)
	}
	k := Kalman(series(5, values...), 1e-9, 0.0001)
	if math.Abs(k.Estimate-values[len(values)-1].(float64)) > 0.02 {
		t.Errorf("estimate %v drifted from the last reading %v", k.Estimate, values[len(values)-1])
	}
	wantRate := -0.01 / 60
	if math.Abs(k.Rate-wantRate) > 2e-5 {
		t.Errorf("rate %v, want about %v", k.Rate, wantRate)
	}
	if k.Surprise > 3 {
		t.Errorf("a jittery but steady drain surprised the filter: %+v", k)
	}
}

func TestKalmanIsSurprisedByAJump(t *testing.T) {
	values := make([]any, 0, 61)
	for range 60 {
		values = append(values, 20.0)
	}
	values = append(values, 18.0) // the tank lost two cubic metres between two polls
	k := Kalman(series(5, values...), 1e-9, 0.0001)
	if k.Surprise < 20 {
		t.Errorf("a 2 m³ jump on a 0.01 m³ sensor should be far beyond 20 sigma: %+v", k)
	}
	if math.Abs(k.Prediction-20) > 0.01 || math.Abs(k.Innovation-(-2)) > 0.01 {
		t.Errorf("prediction %v innovation %v, want 20 and -2", k.Prediction, k.Innovation)
	}
}

func TestKalmanAbsorbsARateChange(t *testing.T) {
	// Fifty samples flat, then bunkering starts at 0.02 m³/s, read each
	// second. The first bunkering sample is a 0.02 m³ step on a sensor
	// with 0.05 m³ noise - well within a sigma - and by the time the trend
	// is established the filter follows it.
	values := make([]any, 0, 110)
	for range 50 {
		values = append(values, 5.0)
	}
	for i := 1; i <= 60; i++ {
		values = append(values, 5.0+0.02*float64(i))
	}
	first := Kalman(series(1, values[:51]...), 1e-5, 0.0025)
	if first.Surprise > 2 {
		t.Errorf("the first bunkering sample should not be a surprise: %+v", first)
	}
	later := Kalman(series(1, values...), 1e-5, 0.0025)
	if later.Surprise > 2 {
		t.Errorf("an established bunkering trend should be followed: %+v", later)
	}
	if later.Rate < 0.01 {
		t.Errorf("rate %v, want close to the 0.02 m³/s bunkering rate", later.Rate)
	}
}

func TestKalmanParametersMayBeInts(t *testing.T) {
	// expr hands an integer literal to a Go function as an int.
	k := Kalman(series(1, 1.0, 1.0, 1.0), 0, 1)
	if k.Samples != 3 || math.IsNaN(k.Surprise) || math.IsInf(k.Sigma, 0) {
		t.Errorf("int parameters: %+v", k)
	}
	// A measurement noise of 0 must not divide by zero.
	k = Kalman(series(1, 1.0, 1.0, 1.0), 0, 0)
	if math.IsNaN(k.Surprise) || math.IsInf(k.Surprise, 0) {
		t.Errorf("zero measurement noise: %+v", k)
	}
}

func TestFreshValue(t *testing.T) {
	fresh := message.SingleValueMapped{Path: "x", Timestamp: time.Now(), Value: 7.5}
	stale := message.SingleValueMapped{Path: "x", Timestamp: time.Now().Add(-2 * time.Minute), Value: 7.5}
	if v := FreshValue(nil, "30s", 0); v != 0 {
		t.Errorf("never reported: %v, want the fallback 0", v)
	}
	if v := FreshValue(fresh, "30s", 0); v != 7.5 {
		t.Errorf("fresh: %v, want 7.5", v)
	}
	if v := FreshValue(&fresh, "30s", 0); v != 7.5 {
		t.Errorf("fresh pointer: %v, want 7.5", v)
	}
	if v := FreshValue(stale, "30s", 0); v != 0 {
		t.Errorf("stale: %v, want the fallback 0", v)
	}
	if v := FreshValue(stale, 300, 0); v != 7.5 {
		t.Errorf("a numeric window is seconds: %v, want 7.5", v)
	}
	if v := FreshValue(fresh, "not a duration", -1); v != -1 {
		t.Errorf("an unparsable window falls back: %v, want -1", v)
	}
	if v := FreshValue("junk", "30s", 0); v != 0 {
		t.Errorf("a non-value falls back: %v, want 0", v)
	}
}

// A sum of two generators where one of them has never answered: with a
// plain a.Value + b.Value the whole mapping errors and the onboard network
// is never published; with freshValue the missing generator counts as 0.
func TestFreshValueSumsOverADeadGenerator(t *testing.T) {
	mappings := []*config.ExpressionMappingConfig{
		{
			MappingConfig: config.MappingConfig{
				Path:       "electrical.ac.onboardNetwork.phase.A.realPower",
				Expression: "freshValue(electrical_ac_generatorFwd_phase_A_realPower, '30s', 0) + freshValue(electrical_ac_generatorAft_phase_A_realPower, '30s', 0)",
			},
			SourcePaths: []string{"electrical.ac.generatorFwd.phase.A.realPower", "electrical.ac.generatorAft.phase.A.realPower"},
		},
	}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.DoMap(message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
		message.NewUpdate().WithSource(
			*message.NewSource().WithLabel("testingConnector").WithType(config.ModbusType).WithUuid(uuid.Nil),
		).WithTimestamp(time.Now()).AddValue(
			message.NewValue().WithPath("electrical.ac.generatorFwd.phase.A.realPower").WithValue(4200.0),
		),
	))
	if err != nil {
		t.Fatal(err)
	}
	var got any
	for _, svm := range result.ToSingleValueMapped() {
		if svm.Path == "electrical.ac.onboardNetwork.phase.A.realPower" {
			got = svm.Value
		}
	}
	if f, ok := got.(float64); !ok || f != 4200 {
		t.Errorf("onboard network with the aft generator dead: %v, want 4200", got)
	}
}

// The functions are only useful if an aggregate expression can reach them
// through history['path'] and read the result's fields, so run one through
// the real mapper.
func TestKalmanThroughAnAggregateExpression(t *testing.T) {
	mappings := []*config.ExpressionMappingConfig{
		{
			MappingConfig: config.MappingConfig{
				Path:       "tanks.fuel.portAft.currentVolume.surprise",
				Expression: "kalman(history['tanks_fuel_portAft_currentVolume'], 1e-5, 0.0025).Surprise",
			},
			SourcePaths:   []string{"tanks.fuel.portAft.currentVolume"},
			RetentionTime: 10 * time.Minute,
		},
		{
			MappingConfig: config.MappingConfig{
				Path:       "tanks.fuel.portAft.currentVolume.change",
				Expression: "delta(history['tanks_fuel_portAft_currentVolume'])",
			},
			SourcePaths:   []string{"tanks.fuel.portAft.currentVolume"},
			RetentionTime: 10 * time.Minute,
		},
	}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatal(err)
	}
	feed := func(value float64) map[string]float64 {
		out := map[string]float64{}
		result, err := m.DoMap(message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
			message.NewUpdate().WithSource(
				*message.NewSource().WithLabel("testingConnector").WithType(config.ModbusType).WithUuid(uuid.Nil),
			).WithTimestamp(time.Now()).AddValue(
				message.NewValue().WithPath("tanks.fuel.portAft.currentVolume").WithValue(value),
			),
		))
		if err != nil {
			t.Fatal(err)
		}
		for _, svm := range result.ToSingleValueMapped() {
			if f, ok := svm.Value.(float64); ok {
				out[svm.Path] = f
			}
		}
		return out
	}
	var last map[string]float64
	for range 30 {
		last = feed(4.0)
	}
	if s := last["tanks.fuel.portAft.currentVolume.surprise"]; s > 1 {
		t.Errorf("a flat tank surprised the filter: %v", s)
	}
	last = feed(2.0)
	if s := last["tanks.fuel.portAft.currentVolume.surprise"]; s < 5 {
		t.Errorf("a 2 m³ jump should be a surprise: %v", s)
	}
	if d := last["tanks.fuel.portAft.currentVolume.change"]; math.Abs(d-(-2)) > 1e-9 {
		t.Errorf("change %v, want -2", d)
	}
}
