package mapper

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type AggregateMapper struct {
	config   config.MapperConfig
	protocol string
	// pathRetentionTime is keyed by source path (dot-separated, as it
	// appears in aggregateMappings - not the underscored form used in
	// env/history), the max RetentionTime among mappings that read that
	// path from history. A path no mapping ever reads history for isn't
	// in this map at all, and Go's zero value for a missing key (0) evicts
	// its buffer down to essentially the latest entry - unlike the single
	// global retention this replaces, a long window one slow-changing
	// path's moving average needs (fuel rate, say) no longer forces every
	// other path's buffer, including a 2kHz sensor's, to hold the same
	// window's worth of history for no reason.
	pathRetentionTime map[string]time.Duration
	aggregateMappings map[string][]*config.ExpressionMappingConfig
	env               ExpressionEnvironment

	// shards, when there's more than one, partitions this mapper's own
	// mappings into independent AggregateMapper instances - each with a
	// disjoint subset of aggregateMappings and its own private env and
	// history, so they share no mutable state with each other. See
	// partitionByPaths for why that's safe: two mappings end up in
	// different shards only when they share no source path, directly or
	// transitively, so neither shard's computation can ever depend on the
	// other's state. Map (unlike DoMap itself, which always runs this
	// mapper's complete, unsharded mappings in one synchronous call - see
	// its own doc comment) uses shards to run unrelated sensors' data
	// concurrently instead of making them wait behind each other on a
	// single goroutine, the situation that motivated sharding at all: see
	// processSharded's doc comment for the incident this fixes.
	shards      []*AggregateMapper
	shardOfPath map[string]int
}

func NewAggregateMapper(c config.MapperConfig, emc []*config.ExpressionMappingConfig) (*AggregateMapper, error) {
	// These are pointers, so the lazy cache in runExpr would have stuck -
	// but compiling here keeps the shards below from racing to write it,
	// and reports a bad expression once at startup.
	for _, mc := range emc {
		precompileMapping(&mc.MappingConfig)
	}

	m := newAggregateMapper(c, emc)

	shardOfPath, shardCount := partitionByPaths(emc, func(mc *config.ExpressionMappingConfig) []string { return mc.SourcePaths })
	if shardCount > 1 {
		byShard := make([][]*config.ExpressionMappingConfig, shardCount)
		for _, mc := range emc {
			if len(mc.SourcePaths) == 0 {
				// never triggered by anything either way - see
				// aggregateMappings, which is keyed by SourcePaths - so
				// there's no shard to put it in, and nowhere it would ever
				// be evaluated from regardless.
				continue
			}
			shard := shardOfPath[mc.SourcePaths[0]] // every one of mc's own paths resolves to the same shard by construction
			byShard[shard] = append(byShard[shard], mc)
		}
		m.shards = make([]*AggregateMapper, shardCount)
		for i, ms := range byShard {
			m.shards[i] = newAggregateMapper(c, ms)
		}
		m.shardOfPath = shardOfPath
	}

	return m, nil
}

func newAggregateMapper(c config.MapperConfig, emc []*config.ExpressionMappingConfig) *AggregateMapper {
	env := NewExpressionEnvironment()
	env["history"] = make(map[string][]message.SingleValueMapped, 0)
	pathRetentionTime := make(map[string]time.Duration)
	mappings := make(map[string][]*config.ExpressionMappingConfig)
	for _, m := range emc {
		for _, s := range m.SourcePaths {
			mappings[s] = append(mappings[s], m)
			if m.RetentionTime > pathRetentionTime[s] {
				pathRetentionTime[s] = m.RetentionTime
			}
		}
	}
	return &AggregateMapper{config: c, protocol: config.SignalKType, pathRetentionTime: pathRetentionTime, aggregateMappings: mappings, env: env}
}

// GetTickerInterval returns the interval on which the mapper should
// additionally be re-evaluated regardless of incoming data, see
// periodicMapper in main.go. Zero disables this.
func (m *AggregateMapper) GetTickerInterval() time.Duration {
	return m.config.Interval
}

func (m *AggregateMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	if len(m.shards) < 2 {
		process(subscriber, publisher, m, false)
		return
	}
	shards := make([]shardedMapper, len(m.shards))
	for i, s := range m.shards {
		shards[i] = s
	}
	processSharded(subscriber, publisher, shards, m.shardOfPath, false)
}

func (m *AggregateMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	s := message.NewSource().WithLabel("signalk").WithType(m.protocol).WithUuid(uuid.Nil)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Time{}) // initialize with empty timestamp instead of hidden now

	overwrites := make(map[string]struct{}, 0)
	historyMap := m.env["history"].(map[string][]message.SingleValueMapped)

	// First pass: fold every relevant value in this single input into env
	// and history, and note which mappings need re-evaluating - without
	// evaluating any of them yet. A mapping with two source paths that
	// both land in the same input (e.g. torque and revolutions from one
	// Manner frame) would otherwise get evaluated once per source path
	// instead of once per input: at a couple of kHz that's real wasted
	// work, and the wasted evaluation runs against a partially-updated env
	// (only the first of the two source paths applied yet), computing a
	// value from one fresh reading and one stale one - u.AddValue's
	// dedup-by-path means it never reaches a caller (the correct
	// evaluation a moment later replaces it before this Update is ever
	// returned), but it's still evaluated for nothing.
	triggered := make(map[*config.ExpressionMappingConfig]struct{})
	for _, svm := range input.ToSingleValueMapped() {
		mappings, ok := m.aggregateMappings[svm.Path]
		if !ok {
			continue
		}
		if svm.Timestamp.After(u.Timestamp) { // take most recent timestamp from relevant data
			u.WithTimestamp(svm.Timestamp)
		}
		u.Source.Uuid = svm.Source.Uuid // take the uuid from the message that updated this value
		path := strings.ReplaceAll(svm.Path, ".", "_")

		// remove old data from buffer
		retention := m.pathRetentionTime[svm.Path]
		for len(historyMap[path]) > 0 && historyMap[path][0].Timestamp.Before(time.Now().Add(-retention)) {
			historyMap[path] = historyMap[path][1:]
		}
		historyMap[path] = append(historyMap[path], svm)

		m.env[path] = svm
		for _, mapping := range mappings {
			triggered[mapping] = struct{}{}
		}
	}

	// Second pass: each triggered mapping now sees every source path this
	// input updated, evaluated exactly once.
	for mapping := range triggered {
		output, err := runExpr(m.env, &mapping.MappingConfig)
		if err == nil {
			if mapping.Overwrite {
				overwrites[mapping.Path] = struct{}{}
			}
			u.AddValue(message.NewValue().WithPath(mapping.Path).WithValue(output))
		}
	}

	if len(u.Values) > 0 {
		return m.removeOverWrites(input, overwrites).AddUpdate(u), nil
	} else {
		return input, nil
	}
}

func (*AggregateMapper) removeOverWrites(input *message.Mapped, overwrites map[string]struct{}) *message.Mapped {
	result := message.NewMapped().WithContext(input.Context).WithOrigin(input.Origin)
	for _, update := range input.Updates {
		u := message.NewUpdate().WithSource(update.Source).WithTimestamp(update.Timestamp)
		for _, value := range update.Values {
			if _, ok := overwrites[value.Path]; !ok {
				u.AddValue(&value)
			}
		}
		if len(u.Values) > 0 {
			result.AddUpdate(u)
		}
	}
	return result
}
