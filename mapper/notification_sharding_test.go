package mapper

import (
	"sync"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
)

func portStarboardChecks() []*config.NotificationMappingConfig {
	return []*config.NotificationMappingConfig{
		{
			MappingConfig: config.MappingConfig{
				Path:       "notifications.propulsion.mainEnginePort.overTorque",
				Expression: "propulsion_mainEnginePort_drive_torque.Value > 1000",
			},
			SourcePaths: []string{"propulsion.mainEnginePort.drive.torque"},
			Message:     "port torque too high",
			State:       "alarm",
		},
		{
			MappingConfig: config.MappingConfig{
				Path:       "notifications.propulsion.mainEngineStarboard.overTorque",
				Expression: "propulsion_mainEngineStarboard_drive_torque.Value > 1000",
			},
			SourcePaths: []string{"propulsion.mainEngineStarboard.drive.torque"},
			Message:     "starboard torque too high",
			State:       "alarm",
		},
	}
}

// TestNotificationMapperShardsForIndependentChecks mirrors
// TestAggregateMapperShardsForIndependentEngines: two checks with disjoint
// source paths (here, two engines that share no path - see
// aggregateForManner/notifyDefaults in the nix repo, which this mirrors)
// must land in different shards, or sharding buys nothing.
func TestNotificationMapperShardsForIndependentChecks(t *testing.T) {
	m, err := NewNotificationMapper(config.MapperConfig{Context: "testingContext"}, portStarboardChecks())
	if err != nil {
		t.Fatalf("NewNotificationMapper: %v", err)
	}
	if got := len(m.shards); got != 2 {
		t.Fatalf("expected 2 shards for two checks with disjoint source paths, got %d", got)
	}
}

// TestNotificationMapperPeriodicOnlyCheckStillSweptWhenSharded is a
// regression test: a check with no source paths at all can only ever be
// evaluated by the periodic sweep (refreshMap), never by DoMap - see
// NewNotificationMapper's periodicOnly handling. Sharding must not
// silently stop sweeping it just because some OTHER, unrelated check
// happens to cause this mapper to shard at all.
func TestNotificationMapperPeriodicOnlyCheckStillSweptWhenSharded(t *testing.T) {
	checks := append(portStarboardChecks(), &config.NotificationMappingConfig{
		MappingConfig: config.MappingConfig{
			Path:       "notifications.test.periodicOnly",
			Expression: "true",
		},
		Message: "periodic-only test message",
		State:   "alarm",
		// SourcePaths deliberately left empty
	})
	m, err := NewNotificationMapper(config.MapperConfig{Context: "testingContext"}, checks)
	if err != nil {
		t.Fatalf("NewNotificationMapper: %v", err)
	}
	if len(m.shards) != 3 {
		t.Fatalf("expected 2 path-based shards plus 1 periodic-only shard, got %d", len(m.shards))
	}

	var periodicShard *NotificationMapper
	for _, shard := range m.shards {
		if len(shard.states) == 1 {
			for nmc := range shard.states {
				if nmc.Path == "notifications.test.periodicOnly" {
					periodicShard = shard
				}
			}
		}
	}
	if periodicShard == nil {
		t.Fatalf("expected to find a shard containing exactly the periodic-only check")
	}

	out := periodicShard.refreshMap(time.Now())
	if len(out.Updates) != 1 {
		t.Fatalf("expected the periodic-only check to be evaluated by its own shard's sweep, got %d updates", len(out.Updates))
	}
}

// TestNotificationMapperShardedMatchesUnshardedOutput mirrors
// TestAggregateMapperShardedMatchesUnshardedOutput: the same input driven
// through the sharded dispatch (runSharded, exactly what Map uses) and
// through a plain sequential DoMap call must raise the same notification.
func TestNotificationMapperShardedMatchesUnshardedOutput(t *testing.T) {
	checks := portStarboardChecks()

	sharded, err := NewNotificationMapper(config.MapperConfig{Context: "testingContext"}, checks)
	if err != nil {
		t.Fatalf("NewNotificationMapper (sharded): %v", err)
	}
	if len(sharded.shards) != 2 {
		t.Fatalf("expected 2 shards, got %d", len(sharded.shards))
	}

	unsharded, err := NewNotificationMapper(config.MapperConfig{Context: "testingContext"}, checks[:1])
	if err != nil {
		t.Fatalf("NewNotificationMapper (unsharded, port only): %v", err)
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
		runSharded(receiveBuffer, sendBuffer, shards, sharded.shardOfPath, true)
	}()

	receiveBuffer <- singleValueUpdate("propulsion.mainEnginePort.drive.torque", 2000, time.Now())
	close(receiveBuffer)
	<-done
	close(sendBuffer)

	var gotNotifying bool
	for out := range sendBuffer {
		for _, svm := range out.ToSingleValueMapped() {
			if svm.Path == "notifications.propulsion.mainEnginePort.overTorque" {
				if _, ok := svm.Value.(message.Notification); ok {
					gotNotifying = true
				}
			}
		}
	}
	if !gotNotifying {
		t.Fatalf("expected the sharded dispatch to raise notifications.propulsion.mainEnginePort.overTorque")
	}

	want, err := unsharded.DoMap(singleValueUpdate("propulsion.mainEnginePort.drive.torque", 2000, time.Now()))
	if err != nil {
		t.Fatalf("unsharded DoMap: %v", err)
	}
	var wantNotifying bool
	for _, svm := range want.ToSingleValueMapped() {
		if svm.Path == "notifications.propulsion.mainEnginePort.overTorque" {
			if _, ok := svm.Value.(message.Notification); ok {
				wantNotifying = true
			}
		}
	}
	if !wantNotifying {
		t.Fatalf("expected the unsharded DoMap to also raise the notification - test setup problem, not a sharding bug")
	}
}

// TestNotificationMapperShardedConcurrencyIsRaceFree hammers both shards -
// including checks each shard evaluates via expr-lang's VM (see runVM) -
// from many concurrent producer goroutines at once, so `go test -race` can
// catch any accidental sharing between shards, or reuse of shared
// expression-evaluation state, that a single-threaded test would never
// exercise. This is the regression test for the actual bug this sharding
// work surfaced: a single package-level vm.VM shared across every runExpr
// call, silently corrupting concurrent evaluations instead of crashing
// (see runVM's doc comment).
func TestNotificationMapperShardedConcurrencyIsRaceFree(t *testing.T) {
	m, err := NewNotificationMapper(config.MapperConfig{Context: "testingContext"}, portStarboardChecks())
	if err != nil {
		t.Fatalf("NewNotificationMapper: %v", err)
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
		runSharded(receiveBuffer, sendBuffer, shards, m.shardOfPath, true)
	}()

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range sendBuffer {
		}
	}()

	paths := []string{
		"propulsion.mainEnginePort.drive.torque",
		"propulsion.mainEngineStarboard.drive.torque",
	}

	var wg sync.WaitGroup
	for _, p := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			for i := range 200 {
				receiveBuffer <- singleValueUpdate(path, float64(i*100), time.Now())
			}
		}(p)
	}
	wg.Wait()
	close(receiveBuffer)
	<-done
	close(sendBuffer)
	<-drained
}
