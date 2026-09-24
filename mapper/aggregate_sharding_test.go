package mapper

import (
	"sync"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
)

func portStarboardMappings() []*config.ExpressionMappingConfig {
	return []*config.ExpressionMappingConfig{
		{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEnginePort.drive.power",
				Expression: "propulsion_mainEnginePort_drive_revolutions.Value * propulsion_mainEnginePort_drive_torque.Value",
			},
			SourcePaths: []string{"propulsion.mainEnginePort.drive.revolutions", "propulsion.mainEnginePort.drive.torque"},
		},
		{
			MappingConfig: config.MappingConfig{
				Path:       "propulsion.mainEngineStarboard.drive.power",
				Expression: "propulsion_mainEngineStarboard_drive_revolutions.Value * propulsion_mainEngineStarboard_drive_torque.Value",
			},
			SourcePaths: []string{"propulsion.mainEngineStarboard.drive.revolutions", "propulsion.mainEngineStarboard.drive.torque"},
		},
	}
}

func singleValueUpdate(path string, value float64, t time.Time) *message.Mapped {
	return message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
		message.NewUpdate().WithSource(
			*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
		).WithTimestamp(t).AddValue(
			message.NewValue().WithPath(path).WithValue(value),
		),
	)
}

// TestAggregateMapperShardsForIndependentEngines verifies that a config
// whose mappings reference two disjoint sets of paths (here, two engines
// that never share a source path - see aggregateForManner in the nix repo,
// which this config mirrors) actually produces more than one shard. This
// is the precondition the rest of the sharding logic (and the CPU-isolation
// it exists for) depends on: if it doesn't hold, sharding buys nothing.
func TestAggregateMapperShardsForIndependentEngines(t *testing.T) {
	mappings := portStarboardMappings()
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}
	if got := len(m.shards); got != 2 {
		t.Fatalf("expected 2 shards for two engines with disjoint source paths, got %d", got)
	}
}

// TestAggregateMapperShardedMatchesUnshardedOutput drives the same input
// sequence through the mapper's sharded dispatch (runSharded, exercising
// exactly the concurrent code path Map uses) and through plain sequential
// DoMap calls on a freshly constructed, unsharded mapper, and checks they
// compute the same aggregated power for the port engine. This is the
// correctness half of sharding: splitting unrelated engines onto separate
// goroutines must not change what gets computed, only how many CPU cores
// it can use while computing it.
func TestAggregateMapperShardedMatchesUnshardedOutput(t *testing.T) {
	mappings := portStarboardMappings()
	now := time.Now()

	sharded, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
	if err != nil {
		t.Fatalf("NewAggregateMapper (sharded): %v", err)
	}
	if len(sharded.shards) != 2 {
		t.Fatalf("expected 2 shards, got %d", len(sharded.shards))
	}

	unsharded, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings[:1])
	if err != nil {
		t.Fatalf("NewAggregateMapper (unsharded, port only): %v", err)
	}

	receiveBuffer := make(chan *message.Mapped, 64)
	sendBuffer := make(chan *message.Mapped, 64)
	shards := make([]shardedMapper, len(sharded.shards))
	for i, s := range sharded.shards {
		shards[i] = s
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSharded(receiveBuffer, sendBuffer, shards, sharded.shardOfPath, false)
	}()

	receiveBuffer <- singleValueUpdate("propulsion.mainEnginePort.drive.revolutions", 100, now)
	receiveBuffer <- singleValueUpdate("propulsion.mainEnginePort.drive.torque", 50, now)
	close(receiveBuffer)
	<-done
	close(sendBuffer)

	var got *message.Mapped
	for out := range sendBuffer {
		for _, svm := range out.ToSingleValueMapped() {
			if svm.Path == "propulsion.mainEnginePort.drive.power" {
				got = out
			}
		}
	}
	if got == nil {
		t.Fatalf("expected a message computing propulsion.mainEnginePort.drive.power, got none")
	}

	if _, err := unsharded.DoMap(singleValueUpdate("propulsion.mainEnginePort.drive.revolutions", 100, now)); err != nil {
		t.Fatalf("unsharded DoMap (revolutions): %v", err)
	}
	want, err := unsharded.DoMap(singleValueUpdate("propulsion.mainEnginePort.drive.torque", 50, now))
	if err != nil {
		t.Fatalf("unsharded DoMap (torque): %v", err)
	}

	var gotPower, wantPower any
	for _, svm := range got.ToSingleValueMapped() {
		if svm.Path == "propulsion.mainEnginePort.drive.power" {
			gotPower = svm.Value
		}
	}
	for _, svm := range want.ToSingleValueMapped() {
		if svm.Path == "propulsion.mainEnginePort.drive.power" {
			wantPower = svm.Value
		}
	}
	if gotPower != wantPower {
		t.Fatalf("sharded result %v does not match unsharded result %v", gotPower, wantPower)
	}
}

// TestAggregateMapperShardedConcurrencyIsRaceFree hammers both shards from
// many concurrent producer goroutines at once - the scenario sharding
// exists for (independent sensors publishing concurrently) - so `go test
// -race` can catch any accidental sharing between shards that a
// single-threaded test would never exercise.
func TestAggregateMapperShardedConcurrencyIsRaceFree(t *testing.T) {
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, portStarboardMappings())
	if err != nil {
		t.Fatalf("NewAggregateMapper: %v", err)
	}
	if len(m.shards) != 2 {
		t.Fatalf("expected 2 shards, got %d", len(m.shards))
	}

	receiveBuffer := make(chan *message.Mapped, 256)
	sendBuffer := make(chan *message.Mapped, 256)
	shards := make([]shardedMapper, len(m.shards))
	for i, s := range m.shards {
		shards[i] = s
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSharded(receiveBuffer, sendBuffer, shards, m.shardOfPath, false)
	}()

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range sendBuffer {
		}
	}()

	paths := []string{
		"propulsion.mainEnginePort.drive.revolutions",
		"propulsion.mainEnginePort.drive.torque",
		"propulsion.mainEngineStarboard.drive.revolutions",
		"propulsion.mainEngineStarboard.drive.torque",
	}

	var wg sync.WaitGroup
	for _, p := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			for i := range 200 {
				receiveBuffer <- singleValueUpdate(path, float64(i), time.Now())
			}
		}(p)
	}
	wg.Wait()
	close(receiveBuffer)
	<-done
	close(sendBuffer)
	<-drained
}
