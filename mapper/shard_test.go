package mapper

import "testing"

type fakeItem struct {
	paths []string
}

func TestPartitionByPathsGroupsSharedPathsTogether(t *testing.T) {
	items := []fakeItem{
		{paths: []string{"a", "b"}},
		{paths: []string{"b", "c"}}, // shares "b" with the first: same shard
		{paths: []string{"x", "y"}}, // disjoint: separate shard
		{paths: []string{"z"}},      // disjoint singleton: its own shard
	}

	shardOfPath, shardCount := partitionByPaths(items, func(i fakeItem) []string { return i.paths })

	if shardCount != 3 {
		t.Fatalf("expected 3 shards, got %d", shardCount)
	}
	if shardOfPath["a"] != shardOfPath["b"] || shardOfPath["b"] != shardOfPath["c"] {
		t.Fatalf("expected a, b, c in the same shard, got %v", shardOfPath)
	}
	if shardOfPath["x"] != shardOfPath["y"] {
		t.Fatalf("expected x, y in the same shard, got %v", shardOfPath)
	}
	if shardOfPath["a"] == shardOfPath["x"] {
		t.Fatalf("expected {a,b,c} and {x,y} in different shards, got %v", shardOfPath)
	}
	if shardOfPath["a"] == shardOfPath["z"] || shardOfPath["x"] == shardOfPath["z"] {
		t.Fatalf("expected z in its own shard, got %v", shardOfPath)
	}
}

func TestPartitionByPathsTransitiveChain(t *testing.T) {
	// a-b, b-c, c-d: all four must end up in one shard even though no
	// single item lists all four together.
	items := []fakeItem{
		{paths: []string{"a", "b"}},
		{paths: []string{"b", "c"}},
		{paths: []string{"c", "d"}},
	}

	shardOfPath, shardCount := partitionByPaths(items, func(i fakeItem) []string { return i.paths })

	if shardCount != 1 {
		t.Fatalf("expected 1 shard, got %d", shardCount)
	}
	for _, p := range []string{"a", "b", "c", "d"} {
		if shardOfPath[p] != shardOfPath["a"] {
			t.Fatalf("expected %q in the same shard as \"a\", got %v", p, shardOfPath)
		}
	}
}

func TestPartitionByPathsEmptyInput(t *testing.T) {
	shardOfPath, shardCount := partitionByPaths([]fakeItem{}, func(i fakeItem) []string { return i.paths })
	if shardCount != 0 || len(shardOfPath) != 0 {
		t.Fatalf("expected no shards for empty input, got count=%d map=%v", shardCount, shardOfPath)
	}
}

func TestPartitionByPathsSinglePathItem(t *testing.T) {
	items := []fakeItem{{paths: []string{"solo"}}}
	shardOfPath, shardCount := partitionByPaths(items, func(i fakeItem) []string { return i.paths })
	if shardCount != 1 {
		t.Fatalf("expected 1 shard, got %d", shardCount)
	}
	if _, ok := shardOfPath["solo"]; !ok {
		t.Fatalf("expected \"solo\" to be assigned a shard, got %v", shardOfPath)
	}
}
