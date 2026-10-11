package mapper

import (
	"math"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
)

// fuelChainMappings is the chained aggregate this feature exists for: a
// fuel rate derived from a supply and a return reading, and a
// fuel-per-distance derived in turn from that fuel rate - a path no
// connector ever publishes, only this same aggregate stage does.
func fuelChainMappings() []*config.ExpressionMappingConfig {
	return []*config.ExpressionMappingConfig{
		{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEngine.fuel.rate",
				Expression: "propulsion_mainEngine_fuel_rate_supply.Value - propulsion_mainEngine_fuel_rate_return.Value",
			},
			SourcePaths: []string{"propulsion.mainEngine.fuel.rate.supply", "propulsion.mainEngine.fuel.rate.return"},
		},
		{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEngine.fuel.fuelToDistance",
				Expression: "propulsion_mainEngine_fuel_rate.Value / navigation_speedOverGround.Value",
			},
			SourcePaths: []string{"propulsion.mainEngine.fuel.rate", "navigation.speedOverGround"},
		},
	}
}

func valueOf(t *testing.T, m *message.Mapped, path string) (float64, bool) {
	t.Helper()
	for _, svm := range m.ToSingleValueMapped() {
		if svm.Path == path {
			f, ok := svm.Value.(float64)
			if !ok {
				t.Fatalf("%s has value %#v, want a float64", path, svm.Value)
			}
			return f, true
		}
	}
	return 0, false
}

func mustValueOf(t *testing.T, m *message.Mapped, path string) float64 {
	t.Helper()
	v, ok := valueOf(t, m, path)
	if !ok {
		t.Fatalf("expected %s in the output, got %+v", path, m)
	}
	return v
}

func multiValueUpdate(t time.Time, pathValues ...any) *message.Mapped {
	u := message.NewUpdate().WithSource(*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType)).WithTimestamp(t)
	for i := 0; i < len(pathValues); i += 2 {
		u.AddValue(message.NewValue().WithPath(pathValues[i].(string)).WithValue(pathValues[i+1]))
	}
	return message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(u)
}

// TestAggregateChainResolvesWithinOneInput is the core of the feature: the
// input carries only the supply and return readings, and a single DoMap
// call has to produce both the fuel rate derived from them and the
// fuel-per-distance derived from that fuel rate. Before mappings were
// evaluated in dependency order, fuelToDistance could only ever see the
// fuel rate computed from some earlier input.
func TestAggregateChainResolvesWithinOneInput(t *testing.T) {
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, fuelChainMappings())
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}

	// speed over ground arrives on its own first; nothing can be derived
	// from it yet, since no fuel rate exists at this point.
	if _, err := m.DoMap(multiValueUpdate(time.Now(), "navigation.speedOverGround", 4.0)); err != nil {
		t.Fatalf("DoMap (speed over ground): %v", err)
	}

	out, err := m.DoMap(multiValueUpdate(time.Now(),
		"propulsion.mainEngine.fuel.rate.supply", 30.0,
		"propulsion.mainEngine.fuel.rate.return", 10.0,
	))
	if err != nil {
		t.Fatalf("DoMap (supply and return): %v", err)
	}

	if got := mustValueOf(t, out, "propulsion.mainEngine.fuel.rate"); got != 20.0 {
		t.Errorf("fuel rate = %v, want 20", got)
	}
	if got := mustValueOf(t, out, "propulsion.mainEngine.fuel.fuelToDistance"); got != 5.0 {
		t.Errorf("fuel to distance = %v, want 5 (a fuel rate of 20 over a speed of 4)", got)
	}
}

// TestAggregateChainUsesLatestDerivedValueOnLaterInput covers the other
// direction: the consumer is triggered by its own non-derived source path
// while the derived one was computed from an earlier input, and has to
// still be readable.
func TestAggregateChainUsesLatestDerivedValueOnLaterInput(t *testing.T) {
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, fuelChainMappings())
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}

	if _, err := m.DoMap(multiValueUpdate(time.Now(),
		"propulsion.mainEngine.fuel.rate.supply", 30.0,
		"propulsion.mainEngine.fuel.rate.return", 10.0,
	)); err != nil {
		t.Fatalf("DoMap (supply and return): %v", err)
	}

	out, err := m.DoMap(multiValueUpdate(time.Now(), "navigation.speedOverGround", 2.0))
	if err != nil {
		t.Fatalf("DoMap (speed over ground): %v", err)
	}

	if got := mustValueOf(t, out, "propulsion.mainEngine.fuel.fuelToDistance"); got != 10.0 {
		t.Errorf("fuel to distance = %v, want 10 (the fuel rate of 20 derived earlier, over a speed of 2)", got)
	}
	// the fuel rate itself wasn't retriggered by this input, so it is not
	// republished - only recomputed paths are.
	if _, ok := valueOf(t, out, "propulsion.mainEngine.fuel.rate"); ok {
		t.Errorf("fuel rate should not be republished when neither of its source paths changed, got %+v", out)
	}
}

// TestAggregateDerivedPathEntersHistory checks that a derived path is
// readable through history, and so through movingAverage, exactly like a
// path arriving from a connector is.
func TestAggregateDerivedPathEntersHistory(t *testing.T) {
	mappings := []*config.ExpressionMappingConfig{
		{
			MappingConfig: config.MappingConfig{Path: "test.derived", Expression: "test_raw.Value * 2"},
			SourcePaths:   []string{"test.raw"},
		},
		{
			MappingConfig: config.MappingConfig{Path: "test.derivedAverage", Expression: "movingAverage(history['test_derived'])"},
			SourcePaths:   []string{"test.derived"},
			RetentionTime: time.Minute,
		},
	}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}

	out, err := m.DoMap(multiValueUpdate(time.Now(), "test.raw", 1.0))
	if err != nil {
		t.Fatalf("DoMap (first): %v", err)
	}
	if got := mustValueOf(t, out, "test.derivedAverage"); got != 2.0 {
		t.Errorf("average over one derived sample = %v, want 2", got)
	}

	if out, err = m.DoMap(multiValueUpdate(time.Now(), "test.raw", 3.0)); err != nil {
		t.Fatalf("DoMap (second): %v", err)
	}
	if got := mustValueOf(t, out, "test.derivedAverage"); got != 4.0 {
		t.Errorf("average over the derived samples 2 and 6 = %v, want 4", got)
	}
}

// TestAggregateRejectsCyclicMappings is the answer to "can this loop
// forever": it can't, because a config whose mappings feed each other in
// a circle is refused while the mapper is being constructed, which
// cmd/map.go turns into a startup failure.
func TestAggregateRejectsCyclicMappings(t *testing.T) {
	for name, mappings := range map[string][]*config.ExpressionMappingConfig{
		"two mappings feeding each other": {
			{
				MappingConfig: config.MappingConfig{Path: "test.a", Expression: "test_b.Value + 1"},
				SourcePaths:   []string{"test.b"},
			},
			{
				MappingConfig: config.MappingConfig{Path: "test.b", Expression: "test_a.Value + 1"},
				SourcePaths:   []string{"test.a"},
			},
		},
		"a transform reading a path derived from its own output": {
			{
				MappingConfig: config.MappingConfig{Path: "test.a", Expression: "test_a.Value + test_b.Value"},
				SourcePaths:   []string{"test.a", "test.b"},
				Overwrite:     true,
			},
			{
				MappingConfig: config.MappingConfig{Path: "test.b", Expression: "test_a.Value * 2"},
				SourcePaths:   []string{"test.a"},
			},
		},
		"a longer circle through a third mapping": {
			{
				MappingConfig: config.MappingConfig{Path: "test.a", Expression: "test_c.Value + 1"},
				SourcePaths:   []string{"test.c"},
			},
			{
				MappingConfig: config.MappingConfig{Path: "test.b", Expression: "test_a.Value + 1"},
				SourcePaths:   []string{"test.a"},
			},
			{
				MappingConfig: config.MappingConfig{Path: "test.c", Expression: "test_b.Value + 1"},
				SourcePaths:   []string{"test.b"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings); err == nil {
				t.Fatal("expected a cyclic aggregate config to be rejected, got no error")
			}
		})
	}
}

// revolutionsTransform is the in-place transform shipped in
// config/mapper/aggregate.yaml: it reads drive.revolutions, sign-corrects
// it from the torque, and republishes it under the same path with
// overwrite: true.
func revolutionsTransform() *config.ExpressionMappingConfig {
	return &config.ExpressionMappingConfig{
		MappingConfig: config.MappingConfig{
			Path:       "propulsion.mainEngine.drive.revolutions",
			Expression: "-copySign(propulsion_mainEngine_drive_revolutions.Value, propulsion_mainEngine_drive_torque.Value)",
		},
		SourcePaths: []string{"propulsion.mainEngine.drive.revolutions", "propulsion.mainEngine.drive.torque"},
		Overwrite:   true,
	}
}

// scaledRevolutions reads drive.revolutions, so it tells raw (10 -> 1000)
// from corrected (-10 -> -1000) apart - powerW, which the real config uses,
// takes an absolute value and couldn't.
func scaledRevolutions() *config.ExpressionMappingConfig {
	return &config.ExpressionMappingConfig{
		MappingConfig: config.MappingConfig{
			Path:       "propulsion.mainEngine.drive.scaled",
			Expression: "propulsion_mainEngine_drive_revolutions.Value * 100",
		},
		SourcePaths: []string{"propulsion.mainEngine.drive.revolutions", "propulsion.mainEngine.drive.torque"},
	}
}

// TestAggregateOverwriteFeedsBackToReaders pins the semantics of an
// in-place transform: the mappings reading the path it corrects see the
// corrected value, not the raw incoming one - and they see it regardless
// of where in the config the transform is listed, because the evaluation
// order follows the dependency, not the file. The transform itself reads
// the raw value (its self-edge is the one edge deliberately not drawn, see
// evaluationOrder), which is what keeps it a single evaluation.
func TestAggregateOverwriteFeedsBackToReaders(t *testing.T) {
	for name, mappings := range map[string][]*config.ExpressionMappingConfig{
		"transform listed before its reader": {revolutionsTransform(), scaledRevolutions()},
		"transform listed after its reader":  {scaledRevolutions(), revolutionsTransform()},
	} {
		t.Run(name, func(t *testing.T) {
			m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
			if err != nil {
				t.Fatalf("NewAggregateMapper: %v", err)
			}

			out, err := m.DoMap(multiValueUpdate(time.Now(),
				"propulsion.mainEngine.drive.revolutions", 10.0,
				"propulsion.mainEngine.drive.torque", 5.0,
			))
			if err != nil {
				t.Fatalf("DoMap: %v", err)
			}

			if got := mustValueOf(t, out, "propulsion.mainEngine.drive.revolutions"); got != -10.0 {
				t.Errorf("overwritten revolutions = %v, want -10 (the transform must read the raw 10, not its own output)", got)
			}
			if got := mustValueOf(t, out, "propulsion.mainEngine.drive.scaled"); got != -1000.0 {
				t.Errorf("scaled = %v, want -1000 (from the corrected revolutions of -10, not the raw 10)", got)
			}
			// and only the corrected value is published under that path:
			// the raw one is stripped by overwrite: true
			n := 0
			for _, svm := range out.ToSingleValueMapped() {
				if svm.Path == "propulsion.mainEngine.drive.revolutions" {
					n++
				}
			}
			if n != 1 {
				t.Errorf("drive.revolutions appears %d times in the output, want exactly once", n)
			}
		})
	}
}

// TestAggregateOverwriteReplacesRawInHistory checks that the corrected
// value of an in-place transform replaces, rather than joins, the raw
// value the same input put in history - otherwise a movingAverage over
// that path would average raw and corrected samples together.
func TestAggregateOverwriteReplacesRawInHistory(t *testing.T) {
	mappings := []*config.ExpressionMappingConfig{
		revolutionsTransform(),
		{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEngine.drive.averageRevolutions",
				Expression: "movingAverage(history['propulsion_mainEngine_drive_revolutions'])",
			},
			SourcePaths:   []string{"propulsion.mainEngine.drive.revolutions"},
			RetentionTime: time.Minute,
		},
	}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}

	out, err := m.DoMap(multiValueUpdate(time.Now(),
		"propulsion.mainEngine.drive.revolutions", 10.0,
		"propulsion.mainEngine.drive.torque", 5.0,
	))
	if err != nil {
		t.Fatalf("DoMap (first): %v", err)
	}
	if got := mustValueOf(t, out, "propulsion.mainEngine.drive.averageRevolutions"); got != -10.0 {
		t.Errorf("average over one corrected sample = %v, want -10 (history must hold the corrected value only, not [10, -10])", got)
	}

	if out, err = m.DoMap(multiValueUpdate(time.Now(),
		"propulsion.mainEngine.drive.revolutions", 20.0,
		"propulsion.mainEngine.drive.torque", 5.0,
	)); err != nil {
		t.Fatalf("DoMap (second): %v", err)
	}
	if got := mustValueOf(t, out, "propulsion.mainEngine.drive.averageRevolutions"); got != -15.0 {
		t.Errorf("average over the corrected samples -10 and -20 = %v, want -15", got)
	}
}

// revolutionsAndTorqueTransforms is the pair the fleet's engine mappings
// ship (aggregateForManner in the nix repo): the revolutions take their
// sign from the torque, the torque takes its sign from the revolutions -
// each from the reading as the sensor delivered it. Two in-place transforms
// reading each other's path, which the mapper has to evaluate without
// treating them as a cycle, and without letting whichever runs second see
// the other's corrected value.
func revolutionsAndTorqueTransforms() (revolutions, torque *config.ExpressionMappingConfig) {
	return &config.ExpressionMappingConfig{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEngine.drive.revolutions",
				Expression: "copySign(propulsion_mainEngine_drive_revolutions.Value, propulsion_mainEngine_drive_torque.Value)",
			},
			SourcePaths: []string{"propulsion.mainEngine.drive.revolutions", "propulsion.mainEngine.drive.torque"},
			Overwrite:   true,
		}, &config.ExpressionMappingConfig{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEngine.drive.torque",
				Expression: "propulsion_mainEngine_drive_revolutions.Value < 0 ? -propulsion_mainEngine_drive_torque.Value : propulsion_mainEngine_drive_torque.Value",
			},
			SourcePaths: []string{"propulsion.mainEngine.drive.revolutions", "propulsion.mainEngine.drive.torque"},
			Overwrite:   true,
		}
}

// TestAggregateTransformsReadingEachOtherSeeArrivedValues pins what two
// in-place transforms correcting each other's path compute: each from the
// values as they arrived, in either config order - so the torque's sign
// follows the *raw* revolutions, not the already sign-corrected ones - while
// a plain mapping reading both paths sees both corrections.
func TestAggregateTransformsReadingEachOtherSeeArrivedValues(t *testing.T) {
	revolutions, torque := revolutionsAndTorqueTransforms()
	product := &config.ExpressionMappingConfig{
		MappingConfig: config.MappingConfig{
			Path:       "propulsion.mainEngine.drive.product",
			Expression: "propulsion_mainEngine_drive_revolutions.Value * propulsion_mainEngine_drive_torque.Value",
		},
		SourcePaths: []string{"propulsion.mainEngine.drive.revolutions", "propulsion.mainEngine.drive.torque"},
	}
	for name, mappings := range map[string][]*config.ExpressionMappingConfig{
		"revolutions first": {revolutions, torque, product},
		"torque first":      {product, torque, revolutions},
	} {
		t.Run(name, func(t *testing.T) {
			m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
			if err != nil {
				t.Fatalf("NewAggregateMapper: %v", err)
			}
			out, err := m.DoMap(multiValueUpdate(time.Now(),
				"propulsion.mainEngine.drive.revolutions", -10.0,
				"propulsion.mainEngine.drive.torque", 5.0,
			))
			if err != nil {
				t.Fatalf("DoMap: %v", err)
			}
			if got := mustValueOf(t, out, "propulsion.mainEngine.drive.revolutions"); got != 10.0 {
				t.Errorf("revolutions = %v, want 10 (the raw -10 given the sign of the raw torque 5)", got)
			}
			if got := mustValueOf(t, out, "propulsion.mainEngine.drive.torque"); got != -5.0 {
				t.Errorf("torque = %v, want -5 (negated because the *raw* revolutions were negative; +5 means it saw the corrected revolutions)", got)
			}
			if got := mustValueOf(t, out, "propulsion.mainEngine.drive.product"); got != -50.0 {
				t.Errorf("product = %v, want -50 (corrected revolutions 10 times corrected torque -5)", got)
			}
		})
	}
}

// TestAggregateTransformOfDerivedPathRunsAfterIt covers a transform applied
// to a path this same aggregate derives rather than a sensor delivers: a
// clamp on a fuel rate computed from the supply and return meters. It has to
// run after the rate is derived (that edge is kept, unlike the ones between
// transforms), read the derived value, and feed the clamped value to the
// mappings reading the rate.
func TestAggregateTransformOfDerivedPathRunsAfterIt(t *testing.T) {
	mappings := []*config.ExpressionMappingConfig{
		{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEngine.fuel.scaled",
				Expression: "propulsion_mainEngine_fuel_rate.Value * 100",
			},
			SourcePaths: []string{"propulsion.mainEngine.fuel.rate"},
		},
		{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEngine.fuel.rate",
				Expression: "propulsion_mainEngine_fuel_rate.Value < 0 ? 0.0 : propulsion_mainEngine_fuel_rate.Value",
			},
			SourcePaths: []string{"propulsion.mainEngine.fuel.rate"},
			Overwrite:   true,
		},
		fuelChainMappings()[0], // the rate from supply and return, listed last on purpose
	}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}
	for _, tc := range []struct{ supply, ret, wantRate, wantScaled float64 }{
		{30, 10, 20, 2000},
		{10, 30, 0, 0}, // the return exceeds the supply, the clamp takes over
	} {
		out, err := m.DoMap(multiValueUpdate(time.Now(),
			"propulsion.mainEngine.fuel.rate.supply", tc.supply,
			"propulsion.mainEngine.fuel.rate.return", tc.ret,
		))
		if err != nil {
			t.Fatalf("DoMap: %v", err)
		}
		if got := mustValueOf(t, out, "propulsion.mainEngine.fuel.rate"); got != tc.wantRate {
			t.Errorf("supply %v return %v: rate = %v, want %v", tc.supply, tc.ret, got, tc.wantRate)
		}
		if got := mustValueOf(t, out, "propulsion.mainEngine.fuel.scaled"); got != tc.wantScaled {
			t.Errorf("supply %v return %v: scaled = %v, want %v (from the clamped rate)", tc.supply, tc.ret, got, tc.wantScaled)
		}
	}
}

// TestAggregateSelfReferenceIsASingleEvaluation covers the simplest
// self-reference - a mapping whose only source path is its own output,
// a counter here - and checks it runs exactly once per input, reading the
// incoming value: self-reference is not a loop.
func TestAggregateSelfReferenceIsASingleEvaluation(t *testing.T) {
	mappings := []*config.ExpressionMappingConfig{{
		MappingConfig: config.MappingConfig{Path: "test.counter", Expression: "test_counter.Value + 1"},
		SourcePaths:   []string{"test.counter"},
		Overwrite:     true,
	}}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}
	out, err := m.DoMap(multiValueUpdate(time.Now(), "test.counter", 41.0))
	if err != nil {
		t.Fatalf("DoMap: %v", err)
	}
	if got := mustValueOf(t, out, "test.counter"); got != 42.0 {
		t.Errorf("counter = %v, want 42 (incremented exactly once)", got)
	}
}

// TestAggregateChainSharesOneShard checks that chaining and sharding stay
// compatible: a derived path and the mapping reading it have to end up in
// the same shard, since a shard only ever evaluates against its own
// private env, while genuinely unrelated mappings must still shard apart
// so the CPU isolation sharding exists for is not lost.
func TestAggregateChainSharesOneShard(t *testing.T) {
	mappings := append(fuelChainMappings(), &config.ExpressionMappingConfig{
		MappingConfig: config.MappingConfig{
			Path:       "propulsion.bowThruster.drive.power",
			Expression: "propulsion_bowThruster_drive_revolutions.Value * propulsion_bowThruster_drive_torque.Value",
		},
		SourcePaths: []string{"propulsion.bowThruster.drive.revolutions", "propulsion.bowThruster.drive.torque"},
	})

	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}
	if got := len(m.shards); got != 2 {
		t.Fatalf("expected the fuel chain and the bow thruster to make 2 shards, got %d", got)
	}
	for _, path := range []string{
		"propulsion.mainEngine.fuel.rate.supply",
		"propulsion.mainEngine.fuel.rate.return",
		"propulsion.mainEngine.fuel.rate",
		"navigation.speedOverGround",
	} {
		if m.shardOfPath[path] != m.shardOfPath["propulsion.mainEngine.fuel.rate"] {
			t.Errorf("%s is in shard %d, want the fuel rate's shard %d", path, m.shardOfPath[path], m.shardOfPath["propulsion.mainEngine.fuel.rate"])
		}
	}

	// and the chain still resolves inside whichever shard it landed in
	fuelShard := m.shards[m.shardOfPath["propulsion.mainEngine.fuel.rate"]]
	if _, err := fuelShard.DoMap(multiValueUpdate(time.Now(), "navigation.speedOverGround", 4.0)); err != nil {
		t.Fatalf("shard DoMap (speed over ground): %v", err)
	}
	out, err := fuelShard.DoMap(multiValueUpdate(time.Now(),
		"propulsion.mainEngine.fuel.rate.supply", 30.0,
		"propulsion.mainEngine.fuel.rate.return", 10.0,
	))
	if err != nil {
		t.Fatalf("shard DoMap (supply and return): %v", err)
	}
	if got := mustValueOf(t, out, "propulsion.mainEngine.fuel.fuelToDistance"); math.Abs(got-5.0) > 1e-9 {
		t.Errorf("fuel to distance inside the shard = %v, want 5", got)
	}
}

// TestShippedAggregateConfigsBuild checks the aggregate configs that ship
// in the repository against the cycle rejection above - a config that
// cannot settle is a startup failure, so it is worth knowing here rather
// than on a vessel.
func TestShippedAggregateConfigsBuild(t *testing.T) {
	for _, path := range []string{"../config/mapper/aggregate.yaml", "../config/mapper/sample-aggregate.yaml"} {
		t.Run(path, func(t *testing.T) {
			if _, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, config.NewExpressionMappingConfig(path)); err != nil {
				t.Fatalf("NewAggregateMapper(%s): %v", path, err)
			}
		})
	}
}
