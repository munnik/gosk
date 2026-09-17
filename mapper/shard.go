package mapper

import (
	"sync"
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

// partitionByPaths groups items into shards, where two items land in the
// same shard iff they share at least one path in common (directly, or
// transitively through a chain of other items). This is what makes sharding
// safe: a mapping only ever needs the paths it lists in sourcePaths(item),
// so two items with disjoint path sets can never need each other's state,
// and can run on independent goroutines with no shared mutable state and
// therefore no locking.
//
// It returns, for every path that appears in any item's paths, which shard
// index (0..shardCount-1) that path belongs to, and the total shard count.
// A path that appears in no item's paths at all is simply absent from the
// returned map - callers treat that as "not part of any shard".
func partitionByPaths[T any](items []T, sourcePaths func(T) []string) (shardOfPath map[string]int, shardCount int) {
	parent := make(map[string]string)

	var find func(string) string
	find = func(p string) string {
		root := p
		for parent[root] != root {
			root = parent[root]
		}
		// path compression
		for parent[p] != root {
			parent[p], p = root, parent[p]
		}
		return root
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	for _, item := range items {
		paths := sourcePaths(item)
		for _, p := range paths {
			if _, ok := parent[p]; !ok {
				parent[p] = p
			}
		}
		for i := 1; i < len(paths); i++ {
			union(paths[0], paths[i])
		}
	}

	shardOfPath = make(map[string]int, len(parent))
	rootShard := make(map[string]int)
	for p := range parent {
		root := find(p)
		shard, ok := rootShard[root]
		if !ok {
			shard = len(rootShard)
			rootShard[root] = shard
		}
		shardOfPath[p] = shard
	}
	return shardOfPath, len(rootShard)
}

// shardedMapper is what processSharded needs from each of a mapper's
// shards: the same contract as RealMapper[message.Mapped], plus
// GetTickerInterval so a shard that itself implements periodicMapper (e.g.
// NotificationMapper) still gets its own independent sweep - scoped to
// just that shard's own state, on that shard's own goroutine, the same way
// a plain, unsharded mapper's sweep already works in process (see
// GetTickerInterval and periodicMapper there).
type shardedMapper interface {
	DoMap(*message.Mapped) (*message.Mapped, error)
	GetTickerInterval() time.Duration
}

// filterMapped returns a copy of input containing only the Updates/Values
// whose Path satisfies keep. Used to split one input across the several
// shards its values touch, and to forward whatever belongs to no shard at
// all - see processSharded's dispatch logic for why an input ever needs
// splitting instead of just being handed to one shard whole.
func filterMapped(input *message.Mapped, keep func(path string) bool) *message.Mapped {
	result := message.NewMapped().WithContext(input.Context).WithOrigin(input.Origin)
	for _, update := range input.Updates {
		u := message.NewUpdate().WithSource(update.Source).WithTimestamp(update.Timestamp)
		for _, value := range update.Values {
			if keep(value.Path) {
				u.AddValue(&value)
			}
		}
		if len(u.Values) > 0 {
			result.AddUpdate(u)
		}
	}
	return result
}

// processSharded is process's concurrent counterpart, for a mapper whose
// own state has been partitioned into independent shards (see
// partitionByPaths) that share no mutable state with each other - each
// runs its own DoMap calls on its own goroutine, so an unrelated shard's
// input never has to wait behind (or contend for a lock with) this one's.
// shardOfPath maps a path to the index into shards it belongs to; a path
// absent from shardOfPath belongs to no shard and is forwarded unchanged
// without ever being evaluated by any of them - the same thing that
// happens to such a path in the unsharded process/DoMap path, where no
// mapping references it, so it always passes through as-is.
func processSharded(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped], shards []shardedMapper, shardOfPath map[string]int, ignoreEmptyUpdates bool) {
	receiveBuffer := make(chan *message.Mapped, bufferSize)
	defer close(receiveBuffer)
	sendBuffer := make(chan *message.Mapped, bufferSize)
	defer close(sendBuffer)

	go subscriber.Receive(receiveBuffer)
	go publisher.Send(sendBuffer)

	runSharded(receiveBuffer, sendBuffer, shards, shardOfPath, ignoreEmptyUpdates)
}

// runSharded is processSharded's core: the shard workers plus the dispatch
// loop feeding them, on plain channels rather than nanomsg's
// Subscriber/Publisher - the seam that makes this concurrency-critical
// logic (dispatch correctness, shard isolation, race-freedom) directly
// testable without a real nanomsg socket. Returns once receiveBuffer is
// closed and every shard has drained and exited.
func runSharded(receiveBuffer <-chan *message.Mapped, sendBuffer chan<- *message.Mapped, shards []shardedMapper, shardOfPath map[string]int, ignoreEmptyUpdates bool) {
	in := make([]chan *message.Mapped, len(shards))
	var wg sync.WaitGroup
	for i, shard := range shards {
		in[i] = make(chan *message.Mapped, bufferSize)
		wg.Add(1)
		go runShard(shard, in[i], sendBuffer, ignoreEmptyUpdates, &wg)
	}

	for msg := range receiveBuffer {
		dispatchToShards(msg, shardOfPath, in, sendBuffer)
	}

	for _, c := range in {
		close(c)
	}
	wg.Wait()
}

// runShard is one shard's entire lifetime: it owns its shard's DoMap calls
// and, when the shard also implements periodicMapper, its own independent
// ticker - mirroring process's single-mapper loop exactly, just scoped to
// one shard's own input channel and state instead of a whole mapper's.
func runShard(shard shardedMapper, in <-chan *message.Mapped, sendBuffer chan<- *message.Mapped, ignoreEmptyUpdates bool, wg *sync.WaitGroup) {
	defer wg.Done()

	var tick <-chan time.Time
	sweeper, canSweep := shard.(periodicMapper)
	tickerInterval := shard.GetTickerInterval()
	if canSweep && tickerInterval > 0 {
		ticker := time.NewTicker(tickerInterval)
		defer ticker.Stop()
		tick = ticker.C
	}

	for {
		select {
		case msg, ok := <-in:
			if !ok {
				return
			}
			out, err := shard.DoMap(msg)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not map the received data",
					zap.Any("Input", msg),
					zap.String("Error", err.Error()),
				)
				continue
			}
			if len(out.Updates) == 0 {
				if !ignoreEmptyUpdates {
					logger.GetLogger().Warn(
						"No updates after mapping the data",
						zap.Any("Input", msg),
						zap.Any("Output", out),
					)
				}
				continue
			}
			sendBuffer <- out
		case now := <-tick:
			if out := sweeper.refreshMap(now); len(out.Updates) > 0 {
				sendBuffer <- out
			}
		}
	}
}

// dispatchToShards routes msg to the shard(s) its values touch. In
// practice a single input almost always belongs to exactly one shard (or
// none) - proxyMapped fans in each upstream publisher's own messages
// unmodified, and a sensor's own mappings are always self-contained within
// one shard (see partitionByPaths) - so those two cases are handled
// directly, without copying msg at all. A message spanning several shards
// is rare, but handled correctly regardless: it's split so each shard only
// ever sees its own paths, and whatever belongs to no shard at all is
// forwarded directly. Splitting can turn one input into several output
// messages, which is fine - gosk's pipeline never guarantees a downstream
// consumer sees values batched the same way they arrived, so this doesn't
// change the data, just how many messages carry it.
func dispatchToShards(msg *message.Mapped, shardOfPath map[string]int, in []chan *message.Mapped, sendBuffer chan<- *message.Mapped) {
	touched := make(map[int]struct{})
	hasUnshared := false
	for _, svm := range msg.ToSingleValueMapped() {
		if shard, ok := shardOfPath[svm.Path]; ok {
			touched[shard] = struct{}{}
		} else {
			hasUnshared = true
		}
	}

	if len(touched) == 0 {
		sendBuffer <- msg
		return
	}
	if len(touched) == 1 && !hasUnshared {
		for shard := range touched {
			in[shard] <- msg
		}
		return
	}

	for shard := range touched {
		filtered := filterMapped(msg, func(path string) bool { return shardOfPath[path] == shard })
		in[shard] <- filtered
	}
	if hasUnshared {
		sendBuffer <- filterMapped(msg, func(path string) bool {
			_, ok := shardOfPath[path]
			return !ok
		})
	}
}
