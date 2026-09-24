package transfer

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/munnik/gosk/database"
	"github.com/munnik/uuid/v5"
)

// A bucket has to contain exactly the uuids whose spelling starts with its
// prefix - no more, since that would send a vessel looking for data in the
// wrong place, and no fewer, since a uuid outside every bucket is one the
// walk can never find.
func TestPrefixBoundsContainTheirPrefix(t *testing.T) {
	base := time.Date(2026, 3, 14, 9, 26, 53, 123456000, time.UTC)

	for i := range 2000 {
		u := uuid.Must(uuid.NewV7AtTimePrecise(base.Add(time.Duration(i) * 7 * time.Millisecond)))
		digits := strings.ReplaceAll(u.String(), "-", "")

		for _, length := range []int{0, 1, 5, 6, 9, 12, 16, 31, 32} {
			prefix := digits[:length]
			lo, hi, err := prefixBounds(prefix)
			if err != nil {
				t.Fatalf("prefixBounds(%q): %v", prefix, err)
			}
			if bytes.Compare(u[:], lo[:]) < 0 || bytes.Compare(u[:], hi[:]) > 0 {
				t.Fatalf("%s is not between %s and %s for prefix %q", u, lo, hi, prefix)
			}
		}
	}
}

// And a uuid must fall outside the bounds of a prefix it does not start
// with, or buckets would overlap and the same data would be compared twice
// while something else went uncompared.
func TestPrefixBoundsExcludeOtherPrefixes(t *testing.T) {
	u := uuid.Must(uuid.NewV7Precise())
	digits := strings.ReplaceAll(u.String(), "-", "")

	// The neighbouring bucket at nine digits, whichever way that is.
	neighbour := digits[:8]
	if digits[8] == '0' {
		neighbour += "1"
	} else {
		neighbour += "0"
	}

	lo, hi, err := prefixBounds(neighbour)
	if err != nil {
		t.Fatalf("prefixBounds(%q): %v", neighbour, err)
	}
	if bytes.Compare(u[:], lo[:]) >= 0 && bytes.Compare(u[:], hi[:]) <= 0 {
		t.Errorf("%s fell inside the bounds of %q, which it does not start with", u, neighbour)
	}
}

func TestPrefixBoundsRejectsNonsense(t *testing.T) {
	if _, _, err := prefixBounds("zz"); err == nil {
		t.Error("a prefix that is not hex was accepted")
	}
	if _, _, err := prefixBounds(strings.Repeat("a", 33)); err == nil {
		t.Error("a prefix longer than a uuid was accepted")
	}
	// An odd number of digits still has to work: nine is the useful depth
	// for a version 7 uuid, and hex decoding wants pairs.
	if _, _, err := prefixBounds("019cebabc"); err != nil {
		t.Errorf("an odd length prefix was rejected: %v", err)
	}
}

func TestSplitLengthAimsAtTheLeafSize(t *testing.T) {
	for _, c := range []struct {
		name   string
		prefix string
		uuids  int64
		want   int
	}{
		{"the root always starts at the same depth", "", 0, rootPrefixLength},
		{"a bucket at the leaf size splits once", "01234", 2000, 6},
		{"a bucket well over it splits wider", "01234", 500000, 7},
		{"but never wider than maxSplit", "01234", 1 << 40, 5 + maxSplit},
		{"an empty bucket still splits once", "01234", 0, 6},
		{"and nothing goes past a whole uuid", strings.Repeat("a", 32), 1 << 20, 32},
	} {
		if got := splitLength(c.prefix, c.uuids, 2000); got != c.want {
			t.Errorf("%s: splitLength(%q, %d) = %d, want %d", c.name, c.prefix, c.uuids, got, c.want)
		}
	}
}

// Descending on a split of one digit has to terminate: each step adds at
// least one digit and nothing goes past 32, so the deepest a walk can get
// is a single uuid.
func TestSplitLengthAlwaysDescends(t *testing.T) {
	prefix := ""
	for range 64 {
		next := splitLength(prefix, 1<<20, 2000)
		if prefix != "" && next <= len(prefix) {
			t.Fatalf("splitLength(%q) = %d did not descend", prefix, next)
		}
		if next == 32 {
			return
		}
		prefix = strings.Repeat("a", next)
	}
	t.Error("the walk did not reach the bottom in 64 steps")
}

func bucket(prefix string, uuids, digest int64) database.UuidBucket {
	return database.UuidBucket{Prefix: prefix, Uuids: uuids, Digest: digest}
}

func TestCompareBuckets(t *testing.T) {
	offered := []database.UuidBucket{
		bucket("019ceba0", 100, 111),
		bucket("019ceba1", 100, 222),
		bucket("019ceba2", 100, 333),
		bucket("019ceba3", 100, 444),
	}
	mine := []database.UuidBucket{
		bucket("019ceba0", 100, 111), // the same
		bucket("019ceba1", 99, 222),  // one short
		bucket("019ceba2", 100, 999), // as many, but not the same ones
		// 019ceba3 missing entirely
		bucket("019ceba4", 100, 555), // something they do not have
	}

	got := compareBuckets(offered, mine)
	want := []string{"019ceba1", "019ceba2", "019ceba3"}
	if len(got) != len(want) {
		t.Fatalf("compareBuckets = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("compareBuckets[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Holding something the vessel does not is not a gap. Retention runs at
// different times on the two sides, so the far end keeping data a vessel
// has already dropped is normal - and asking the vessel to descend into it
// would send it looking for rows it cannot produce, forever.
func TestCompareBucketsIgnoresWhatOnlyWeHave(t *testing.T) {
	got := compareBuckets(
		[]database.UuidBucket{bucket("019ceba0", 10, 1)},
		[]database.UuidBucket{bucket("019ceba0", 10, 1), bucket("019cea00", 999, 2)},
	)
	if len(got) != 0 {
		t.Errorf("compareBuckets = %v, want nothing to descend into", got)
	}
}

func TestCompareLeaves(t *testing.T) {
	a := uuid.Must(uuid.NewV7Precise())
	b := uuid.Must(uuid.NewV7Precise())
	c := uuid.Must(uuid.NewV7Precise())
	d := uuid.Must(uuid.NewV7Precise())

	offered := []database.UuidCount{
		{Uuid: a, Rows: 3}, // we have all of it
		{Uuid: b, Rows: 3}, // we have some of it
		{Uuid: c, Rows: 3}, // we have none of it
		{Uuid: d, Rows: 3}, // we somehow have more of it
	}
	mine := []database.UuidCount{
		{Uuid: a, Rows: 3},
		{Uuid: b, Rows: 1},
		{Uuid: d, Rows: 4},
	}

	got := compareLeaves(offered, mine)
	if len(got) != 2 || got[0] != b || got[1] != c {
		t.Fatalf("compareLeaves = %v, want the partial one and the absent one", got)
	}
}

func TestDigestOfferRoundTrips(t *testing.T) {
	// The offers and replies go through the same codec as a shipment, and
	// a bucket prefix is a string of hex digits with meaning: losing its
	// leading zeros, say, would point the far end at the wrong range.
	c := newOutboxCodec(true)
	original := OutboxDigestOffer{
		Exchange: uuid.Must(uuid.NewV7Precise()),
		Origin:   "vessels.urn:mrn:imo:mmsi:111111111",
		Prefix:   "019ce",
		Buckets: []database.UuidBucket{
			bucket("019ce0", 12345, -8927187639717044313),
			bucket("019ce1", 1, 0),
		},
	}

	payload, err := c.encode(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded OutboxDigestOffer
	if err := c.decode(payload, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Exchange != original.Exchange || decoded.Prefix != original.Prefix {
		t.Error("the header did not survive")
	}
	if len(decoded.Buckets) != len(original.Buckets) {
		t.Fatalf("%d buckets, want %d", len(decoded.Buckets), len(original.Buckets))
	}
	for i := range original.Buckets {
		if decoded.Buckets[i] != original.Buckets[i] {
			t.Errorf("bucket %d became %+v, want %+v", i, decoded.Buckets[i], original.Buckets[i])
		}
	}
}

// What the scheme is for: a summary has to be far smaller than the keys it
// summarises, or there is no point not just sending the keys.
func TestDigestIsSmallerThanTheKeysItSummarises(t *testing.T) {
	const uuids = 20000

	base := time.Date(2026, 3, 14, 9, 26, 53, 0, time.UTC)
	leaves := make([]database.UuidCount, 0, uuids)
	for i := range uuids {
		leaves = append(leaves, database.UuidCount{
			Uuid: uuid.Must(uuid.NewV7AtTimePrecise(base.Add(time.Duration(i) * 100 * time.Millisecond))),
			Rows: 3,
		})
	}

	// The same span summarised at nine digits: about four seconds a
	// bucket, so a little over five hundred buckets for these.
	buckets := map[string]database.UuidBucket{}
	for _, l := range leaves {
		p := strings.ReplaceAll(l.Uuid.String(), "-", "")[:9]
		b := buckets[p]
		b.Prefix = p
		b.Uuids++
		b.Digest ^= int64(l.Uuid[8])<<56 | int64(l.Uuid[9])<<48
		buckets[p] = b
	}
	summary := make([]database.UuidBucket, 0, len(buckets))
	for _, b := range buckets {
		summary = append(summary, b)
	}

	c := newOutboxCodec(true)
	named, err := c.encode(OutboxDigestOffer{Origin: "o", Leaves: leaves})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	summarised, err := c.encode(OutboxDigestOffer{Origin: "o", Buckets: summary})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	t.Logf("%d source messages: %d bytes named one by one, %d bytes as %d summaries, %.0fx",
		uuids, len(named), len(summarised), len(summary), float64(len(named))/float64(len(summarised)))

	if len(summarised)*4 > len(named) {
		t.Errorf("summarising %d source messages into %d buckets saved almost nothing: %d bytes against %d",
			uuids, len(summary), len(summarised), len(named))
	}
}
