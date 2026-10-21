package mapper

import (
	"fmt"
	"slices"
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
	// aggregateMappings is keyed by source path: the mappings that read
	// that path, and so the mappings a new value for it triggers. A path
	// this same aggregate computes is looked up here exactly like a path
	// arriving from a sensor is, which is what lets one mapping of an
	// aggregate feed another.
	aggregateMappings map[string][]*config.ExpressionMappingConfig
	// evalOrder is every mapping of this mapper, ordered so that the
	// mapping computing a path always comes before the mappings reading
	// it - see evaluationOrder, and DoMap for what that ordering buys.
	evalOrder []*config.ExpressionMappingConfig
	env       ExpressionEnvironment

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

// isTransform reports whether mc is an in-place transform: a mapping that
// lists its own output path among its source paths, and so rewrites a path
// (sign-correcting drive.revolutions from the torque, clamping a rate)
// rather than deriving a new one. Such a mapping reads every path that
// arrived in the input as it arrived - its own, and any other transform's -
// see evaluationOrder and DoMap for what that buys and how it's arranged.
func isTransform(mc *config.ExpressionMappingConfig) bool {
	return mc.Path != "" && slices.Contains(mc.SourcePaths, mc.Path)
}

// evaluationOrder returns emc ordered so that the mapping computing a path
// always comes before every mapping reading that path. consumersOfPath is
// aggregateMappings, i.e. keyed by source path.
//
// That ordering is what lets a chain of aggregates resolve within a single
// DoMap call: fuel.rate computed from its supply and return readings, and
// then - in the same pass, from the value just computed - a fuelToDistance
// that reads fuel.rate. It equally fixes the order of an in-place
// transform against its readers: propulsion.mainEngine.drive.revolutions
// in config/mapper/aggregate.yaml sign-corrects that path with
// overwrite: true, and engineLoad and drive.power, which both read it, are
// guaranteed to run after the correction and so to see the corrected
// value - not whichever value the evaluation happened to reach them with.
// Mappings that don't depend on each other keep their config order, so
// what an input produces never depends on Go's map iteration order, and
// moving an in-place transform further down the config file cannot change
// the result.
//
// The edges deliberately not drawn are the ones between in-place
// transforms (see isTransform). A transform lists its own output path
// among its sourcePaths and reads it as it arrived rather than as the
// value it is about to write - that self-edge is what would otherwise make
// every transform a self-feeding loop. The same goes for a transform
// reading a path *another* transform corrects: it reads that path as it
// arrived too (DoMap arranges that), so there is no dependency between the
// two in either direction. The shipped engine mappings need exactly this -
// drive.revolutions takes its sign from the torque and drive.torque takes
// its sign from the revolutions, each from the reading as the sensor
// delivered it - and drawing those edges would turn every engine config in
// the fleet into a cycle. A transform reading a path a *non*-transform
// computes does depend on it (a clamp applied to a derived fuel rate has to
// run after the rate is derived), so that edge is kept, as is the edge from
// any transform to every non-transform reading its path.
//
// There is deliberately no repeated or fixpoint iteration anywhere in
// this, and therefore no way for a chain of aggregates to loop: every
// mapping is evaluated at most once per input, and a config in which that
// isn't enough - a mapping that reads, through one or more other mappings,
// a path computed from its own output - is a cycle, which is rejected here
// while the mapper is being constructed. cmd/map.go turns the error into a
// startup failure naming the mappings involved, so such a config never
// runs at all rather than live-locking a mapper once it's live.
func evaluationOrder(emc []*config.ExpressionMappingConfig, consumersOfPath map[string][]*config.ExpressionMappingConfig) ([]*config.ExpressionMappingConfig, error) {
	indexOf := make(map[*config.ExpressionMappingConfig]int, len(emc))
	for i, mc := range emc {
		indexOf[mc] = i
	}

	consumers := make([][]int, len(emc))
	indegree := make([]int, len(emc))
	for i, mc := range emc {
		if mc.Path == "" {
			continue // nothing to compute, so nothing can read it either
		}
		for _, consumer := range consumersOfPath[mc.Path] {
			j, ok := indexOf[consumer]
			if !ok || j == i || (isTransform(mc) && isTransform(consumer)) {
				continue // see the doc comment for the edges not drawn
			}
			consumers[i] = append(consumers[i], j)
			indegree[j]++
		}
	}

	// Kahn's algorithm, always taking the lowest-numbered mapping that is
	// ready rather than whatever a queue happens to hand back, so the
	// result is the config order wherever the dependencies allow it.
	order := make([]*config.ExpressionMappingConfig, 0, len(emc))
	emitted := make([]bool, len(emc))
	for len(order) < len(emc) {
		next := -1
		for i := range emc {
			if !emitted[i] && indegree[i] == 0 {
				next = i
				break
			}
		}
		if next == -1 {
			stuck := make([]string, 0, len(emc)-len(order))
			for i, mc := range emc {
				if !emitted[i] {
					stuck = append(stuck, fmt.Sprintf("%s from [%s]", mc.Path, strings.Join(mc.SourcePaths, " ")))
				}
			}
			return nil, fmt.Errorf("the aggregate mappings contain a cycle, each of these mappings depends on a path derived from its own output so no finite number of evaluations could settle them: %s", strings.Join(stuck, "; "))
		}
		emitted[next] = true
		order = append(order, emc[next])
		for _, j := range consumers[next] {
			indegree[j]--
		}
	}
	return order, nil
}

func NewAggregateMapper(c config.MapperConfig, emc []*config.ExpressionMappingConfig) (*AggregateMapper, error) {
	// These are pointers, so the lazy cache in runExpr would have stuck -
	// but compiling here keeps the shards below from racing to write it,
	// and reports a bad expression once at startup.
	for _, mc := range emc {
		precompileMapping(&mc.MappingConfig)
	}

	m, err := newAggregateMapper(c, emc)
	if err != nil {
		return nil, err
	}

	// A path that one mapping computes and another reads has to end up in
	// the same shard as both of them: a shard evaluates its own mappings
	// against its own private env, so a reader sitting in a different
	// shard would look for that path in an env nothing ever writes it to.
	// Tying a mapping's output path to its source paths here is what keeps
	// the two together. For an output path no other mapping reads, this
	// adds nothing at all - hence the `consumed` check - so configs
	// without chained aggregates shard exactly as they did before.
	consumed := make(map[string]struct{})
	for _, mc := range emc {
		for _, s := range mc.SourcePaths {
			consumed[s] = struct{}{}
		}
	}
	shardOfPath, shardCount := partitionByPaths(emc, func(mc *config.ExpressionMappingConfig) []string {
		if _, ok := consumed[mc.Path]; !ok {
			return mc.SourcePaths
		}
		return append(slices.Clone(mc.SourcePaths), mc.Path)
	})
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
			// a subset of an acyclic set of mappings can't contain a cycle
			// the whole didn't, so this can't fail where the mapper above
			// succeeded - it's propagated rather than ignored all the same.
			if m.shards[i], err = newAggregateMapper(c, ms); err != nil {
				return nil, err
			}
		}
		m.shardOfPath = shardOfPath
	}

	return m, nil
}

func newAggregateMapper(c config.MapperConfig, emc []*config.ExpressionMappingConfig) (*AggregateMapper, error) {
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
	order, err := evaluationOrder(emc, mappings)
	if err != nil {
		return nil, err
	}
	return &AggregateMapper{config: c, protocol: config.SignalKType, pathRetentionTime: pathRetentionTime, aggregateMappings: mappings, evalOrder: order, env: env}, nil
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
	appended := make(map[string]int)
	arrived := make(map[string]message.SingleValueMapped)
	for _, svm := range input.ToSingleValueMapped() {
		mappings, ok := m.aggregateMappings[svm.Path]
		if !ok {
			continue
		}
		if svm.Timestamp.After(u.Timestamp) { // take most recent timestamp from relevant data
			u.WithTimestamp(svm.Timestamp)
		}
		u.Source.Uuid = svm.Source.Uuid // take the uuid from the message that updated this value
		m.remember(historyMap, appended, svm, false)
		arrived[svm.Path] = svm
		for _, mapping := range mappings {
			triggered[mapping] = struct{}{}
		}
	}

	// Second pass: each triggered mapping now sees every source path this
	// input updated, evaluated exactly once - in dependency order (see
	// evaluationOrder), so a mapping reading a path another mapping of
	// this same aggregate computes sees the value computed from *this*
	// input rather than the previous one's. Whatever a mapping computes is
	// folded straight back into env and history, and the mappings reading
	// that path are triggered in turn; they are guaranteed to still be
	// ahead of this loop, so a chain of any depth - supply and return to a
	// fuel rate, that fuel rate to a fuel-per-distance - settles in this
	// one pass, with every mapping evaluated at most once and nothing
	// re-run.
	//
	// An in-place transform (isTransform: a mapping whose output path is one
	// of its own source paths, drive.revolutions sign-corrected from the
	// torque being the shipped example) reads the paths that arrived in
	// this input as they arrived - its own, and any other transform's
	// output path. From here on env, history and every later non-transform
	// mapping hold and see the corrected value - the same value the rest of
	// the pipeline sees, since overwrite: true strips the raw one from the
	// output. Two transforms can therefore correct each other's paths from
	// the raw readings (the shipped torque/revolutions pair does), and run
	// in config order; whichever runs second finds the other's corrected
	// value already in env, which is why the arrived values are swapped
	// back in around its evaluation. Re-triggering a transform's own path
	// is harmless: the only mapping that could be is a transform of that
	// path, which either already ran or reads the path as it arrived.
	transformed := make(map[string]struct{})
	for _, mapping := range m.evalOrder {
		if _, ok := triggered[mapping]; !ok {
			continue
		}
		var output any
		var err error
		if isTransform(mapping) {
			output, err = m.runTransform(mapping, arrived, transformed)
			transformed[mapping.Path] = struct{}{}
		} else {
			output, err = runExpr(m.env, &mapping.MappingConfig)
		}
		if err != nil {
			continue
		}
		if mapping.Overwrite {
			overwrites[mapping.Path] = struct{}{}
		}
		// An in-place transform corrects a reading rather than deriving a
		// new one, so the reading's diagnostics - the raw loop current,
		// or why it was rejected - stay with the corrected value. Taken
		// from env, which holds the reading as it arrived or as corrected
		// before (and runTransform has restored by now), so they are
		// there whichever of a transform's source paths triggered it. A
		// derived value has none of its own.
		var diagnostics map[string]any
		if isTransform(mapping) {
			if current, ok := m.env[strings.ReplaceAll(mapping.Path, ".", "_")].(message.SingleValueMapped); ok {
				diagnostics = current.Diagnostics
			}
		}
		switch mapping.Path {
		case "name":
			name := output.(string)
			u.AddValue(message.NewValue().WithPath("").WithValue(message.VesselInfo{Name: &name}))
		case "mmsi":
			mmsi := output.(string)
			u.AddValue(message.NewValue().WithPath("").WithValue(message.VesselInfo{MMSI: &mmsi}))
		default:
			u.AddValue(message.NewValue().WithPath(mapping.Path).WithValue(output).WithDiagnostics(diagnostics))
		}

		if mapping.Path == "" {
			continue
		}
		m.remember(historyMap, appended, message.SingleValueMapped{
			Context:     input.Context,
			Origin:      input.Origin,
			Source:      u.Source,
			Timestamp:   u.Timestamp,
			Path:        mapping.Path,
			Value:       output,
			Diagnostics: diagnostics,
		}, true)
		for _, consumer := range m.aggregateMappings[mapping.Path] {
			triggered[consumer] = struct{}{}
		}
	}

	if len(u.Values) > 0 {
		return m.removeOverWrites(input, overwrites).AddUpdate(u), nil
	} else {
		return input, nil
	}
}

// runTransform evaluates an in-place transform against the values of its
// source paths as they arrived in the current input. Only a path that both
// arrived in this input and has since been rewritten by another transform
// (transformed) can differ from what env holds, so only those are swapped
// for the arrived value for the duration of the evaluation and put back
// afterwards - every later non-transform reader keeps seeing the corrected
// value. A path a transform reads that no transform rewrote, or that a
// non-transform mapping derived rather than a sensor delivered, is read
// from env as any other mapping would read it.
func (m *AggregateMapper) runTransform(mapping *config.ExpressionMappingConfig, arrived map[string]message.SingleValueMapped, transformed map[string]struct{}) (any, error) {
	type swapped struct {
		key   string
		value any
	}
	var restore []swapped
	for _, sourcePath := range mapping.SourcePaths {
		if _, ok := transformed[sourcePath]; !ok {
			continue
		}
		raw, ok := arrived[sourcePath]
		if !ok {
			continue
		}
		key := strings.ReplaceAll(sourcePath, ".", "_")
		restore = append(restore, swapped{key, m.env[key]})
		m.env[key] = raw
	}
	output, err := runExpr(m.env, &mapping.MappingConfig)
	for _, r := range restore {
		m.env[r.key] = r.value
	}
	return output, err
}

// remember makes svm the current value of its path in env and adds it to
// that path's history buffer, first dropping whatever has fallen outside
// the retention window the mappings reading the path asked for (see
// pathRetentionTime). It's used both for a value arriving in an input and
// for one a mapping of this same aggregate computed, so a computed path is
// readable from an expression - directly and through history,
// movingAverage included - in exactly the same way a sensor's path is.
//
// appended counts, per path, the entries the current input has already
// added to history. A computed value for a path the same input also
// carried - an in-place transform's corrected value, over the raw reading
// the first pass folded in - replaces those entries rather than joining
// them: a movingAverage over that path would otherwise average raw and
// corrected samples together, and the raw one is exactly what the
// transform exists to get rid of. The entries to replace are always the
// newest ones (this input's), so dropping them from the tail is right;
// the retention eviction above may already have removed some or all of
// them (retentionTime 0 evicts a path's previous entry as soon as real
// time moves past its timestamp), hence the min.
func (m *AggregateMapper) remember(historyMap map[string][]message.SingleValueMapped, appended map[string]int, svm message.SingleValueMapped, computed bool) {
	path := strings.ReplaceAll(svm.Path, ".", "_")
	cutoff := time.Now().Add(-m.pathRetentionTime[svm.Path])
	for len(historyMap[path]) > 0 && historyMap[path][0].Timestamp.Before(cutoff) {
		historyMap[path] = historyMap[path][1:]
	}
	if computed {
		if drop := min(appended[svm.Path], len(historyMap[path])); drop > 0 {
			historyMap[path] = historyMap[path][:len(historyMap[path])-drop]
		}
		appended[svm.Path] = 0
	}
	historyMap[path] = append(historyMap[path], svm)
	appended[svm.Path]++
	m.env[path] = svm
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
