package transfer

import (
	"github.com/google/uuid"
	"testing"
	"time"

	"github.com/munnik/gosk/database"
	"github.com/munnik/gosk/message"
)

func TestCodecRoundTripsCompressedAndPlain(t *testing.T) {
	ack := Ack{Origin: "vessels.1", Bucket: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), Generation: 7, Through: time.Date(2026, 9, 1, 12, 0, 59, 0, time.UTC), Stored: 3}

	for _, compress := range []bool{true, false} {
		payload, err := newCodec(compress).encode(ack)
		if err != nil {
			t.Fatalf("encode(compress=%v): %v", compress, err)
		}
		// The receiving end does not know the sender's setting.
		var decoded Ack
		if err := newCodec(!compress).decode(payload, &decoded); err != nil {
			t.Fatalf("decode(compress=%v): %v", compress, err)
		}
		if decoded.Origin != ack.Origin || !decoded.Bucket.Equal(ack.Bucket) || decoded.Generation != ack.Generation || !decoded.Through.Equal(ack.Through) || decoded.Stored != ack.Stored {
			t.Fatalf("round trip (compress=%v) changed the ack: %+v", compress, decoded)
		}
	}
}

func TestGroupRowsFoldsOneUpdatePerContextTimeAndSource(t *testing.T) {
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	row := func(context, path string, at time.Time, label string) message.SingleValueMapped {
		return message.SingleValueMapped{
			Context:   context,
			Origin:    "vessels.1",
			Source:    message.Source{Label: label, Type: "nmea0183"},
			Timestamp: at,
			Path:      path,
			Value:     1.0,
		}
	}
	rows := []message.SingleValueMapped{
		row("vessels.1", "a", at, "gps"),
		row("vessels.1", "b", at, "gps"),
		row("vessels.1", "c", at.Add(time.Second), "gps"),
		row("vessels.1", "a", at, "ais"),
		row("vessels.2", "a", at, "gps"),
	}

	grouped := groupRows(rows)

	if len(grouped) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(grouped), grouped)
	}
	if len(grouped[0].Updates) != 1 || len(grouped[0].Updates[0].Values) != 2 {
		t.Fatalf("expected the first message to carry a and b in one update, got %+v", grouped[0])
	}
	total := 0
	for _, m := range grouped {
		for _, u := range m.Updates {
			total += len(u.Values)
		}
	}
	if total != len(rows) {
		t.Fatalf("grouping lost rows: %d of %d", total, len(rows))
	}
}

// TestGroupRowsKeepsRowsWithDifferentSourceUuidsApart: an update carries
// its source's uuids once, for every value in it, so rows from two raw
// messages that happen to share a context, a timestamp and a source must
// not be folded into one - the second would arrive on the far end under
// the first one's uuid.
func TestGroupRowsKeepsRowsWithDifferentSourceUuidsApart(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	first, second := uuid.New(), uuid.New()
	row := func(path string, u uuid.UUID) message.SingleValueMapped {
		return message.SingleValueMapped{
			Context:   "vessels.urn:mrn:imo:mmsi:111111111",
			Origin:    "vessels.urn:mrn:imo:mmsi:111111111",
			Timestamp: at,
			Source:    *message.NewSource().WithLabel("engine").WithType("modbus").WithUuid(u),
			Path:      path,
			Value:     1.0,
		}
	}
	grouped := groupRows([]message.SingleValueMapped{row("a", first), row("b", second), row("c", first)})
	if len(grouped) != 2 {
		t.Fatalf("expected one message per source uuid, got %d: %+v", len(grouped), grouped)
	}
	for _, m := range grouped {
		for _, v := range m.Updates[0].Values {
			want := first
			if v.Path == "b" {
				want = second
			}
			if m.Updates[0].Source.Uuid != want {
				t.Fatalf("row %s arrived under uuid %s, want %s", v.Path, m.Updates[0].Source.Uuid, want)
			}
		}
	}
}

func TestGroupRowsKeepsANameAndAnMMSIRowApart(t *testing.T) {
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	name, mmsi := "Testvessel", "244000001"
	// What ReadOutboxDelivery hands over: the stored paths "name" and
	// "mmsi" both read back as the empty path VesselInfo is written with.
	rows := []message.SingleValueMapped{
		{Context: "vessels.1", Origin: "vessels.1", Source: message.Source{Label: "ais", Type: "nmea0183"}, Timestamp: at, Path: "", Value: message.VesselInfo{Name: &name}},
		{Context: "vessels.1", Origin: "vessels.1", Source: message.Source{Label: "ais", Type: "nmea0183"}, Timestamp: at, Path: "", Value: message.VesselInfo{MMSI: &mmsi}},
	}

	for _, order := range [][]int{{0, 1}, {1, 0}} {
		ordered := []message.SingleValueMapped{rows[order[0]], rows[order[1]]}
		grouped := groupRows(ordered)
		if len(grouped) != 1 || len(grouped[0].Updates) != 1 {
			t.Fatalf("expected one message with one update, got %+v", grouped)
		}
		values := grouped[0].Updates[0].Values
		if len(values) != 2 {
			t.Fatalf("expected both rows to survive grouping, got %+v", values)
		}
		// And the far end stores them under the same two keys the vessel
		// had, so the digests of the two agree afterwards.
		stored := map[string]bool{}
		for _, v := range values {
			vi := v.Value.(message.VesselInfo)
			if vi.MMSI == nil {
				stored["name"] = true
			} else {
				stored["mmsi"] = true
			}
		}
		if !stored["name"] || !stored["mmsi"] {
			t.Fatalf("expected a name and an mmsi row, got %+v", values)
		}
	}
}

func TestCompareDigests(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	offered := []database.DigestBucket{
		{Start: day(1), Rows: 100, Hash: 1}, // identical
		{Start: day(2), Rows: 100, Hash: 2}, // same count, different rows
		{Start: day(3), Rows: 100, Hash: 3}, // far end holds 10%
		{Start: day(4), Rows: 100, Hash: 4}, // far end holds nothing
		{Start: day(5), Rows: 100, Hash: 5}, // far end holds 50%
	}
	mine := []database.DigestBucket{
		{Start: day(1), Rows: 100, Hash: 1},
		{Start: day(2), Rows: 100, Hash: 22},
		{Start: day(3), Rows: 10, Hash: 33},
		{Start: day(5), Rows: 50, Hash: 55},
		{Start: day(6), Rows: 100, Hash: 6}, // only here: not a gap
	}

	missing, differ := compareDigests(offered, mine, 0.25)

	want := func(name string, got []time.Time, days ...int) {
		t.Helper()
		if len(got) != len(days) {
			t.Fatalf("%s: expected days %v, got %v", name, days, got)
		}
		for i, d := range days {
			if !got[i].Equal(day(d)) {
				t.Fatalf("%s: expected days %v, got %v", name, days, got)
			}
		}
	}
	want("missing", missing, 3, 4)
	want("differ", differ, 2, 5)
}

func TestLevelsEndAtTheOutboxBucket(t *testing.T) {
	width := digestLevels[0]
	seen := []time.Duration{width}
	for {
		next, ok := nextLevel(width)
		if !ok {
			break
		}
		if next >= width {
			t.Fatalf("levels must narrow: %v after %v", next, width)
		}
		width = next
		seen = append(seen, width)
	}
	if width != database.OutboxBucket {
		t.Fatalf("the walk must end at the outbox bucket, ended at %v (levels %v)", width, seen)
	}
}

func TestAlignRangeWidensToWholeBuckets(t *testing.T) {
	r := alignRange(TimeRange{
		From: time.Date(2026, 9, 1, 13, 7, 0, 0, time.UTC),
		To:   time.Date(2026, 9, 3, 0, 0, 1, 0, time.UTC),
	}, 24*time.Hour)
	if !r.From.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || !r.To.Equal(time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected alignment: %+v", r)
	}
	// Already aligned: unchanged.
	r = alignRange(r, 24*time.Hour)
	if !r.From.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || !r.To.Equal(time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("aligning twice changed the range: %+v", r)
	}
}

func TestChunkOffersKeepsRangesWholeAndBounded(t *testing.T) {
	hour := func(h int) time.Time { return time.Date(2026, 9, 1, h, 0, 0, 0, time.UTC) }
	ranges := []TimeRange{
		{From: hour(0), To: hour(1)},
		{From: hour(1), To: hour(2)},
		{From: hour(5), To: hour(6)}, // nothing in it
		{From: hour(6), To: hour(7)},
	}
	buckets := []database.DigestBucket{}
	for _, h := range []int{0, 1, 6} {
		for m := 0; m < 60; m++ {
			buckets = append(buckets, database.DigestBucket{Start: hour(h).Add(time.Duration(m) * time.Minute), Rows: 1})
		}
	}

	offers := chunkOffers(ranges, buckets, time.Minute, 100)

	if len(offers) != 3 {
		t.Fatalf("expected 3 offers (60, 60, 0+60 buckets), got %d", len(offers))
	}
	seenRanges, seenBuckets := 0, 0
	for _, o := range offers {
		if len(o.Buckets) > 100 {
			t.Fatalf("an offer exceeds the bound: %d buckets", len(o.Buckets))
		}
		if o.Width != time.Minute {
			t.Fatalf("offer lost its width: %v", o.Width)
		}
		seenRanges += len(o.Ranges)
		seenBuckets += len(o.Buckets)
	}
	if seenRanges != len(ranges) || seenBuckets != len(buckets) {
		t.Fatalf("chunking lost something: %d/%d ranges, %d/%d buckets", seenRanges, len(ranges), seenBuckets, len(buckets))
	}
	if len(offers[1].Ranges) != 2 || len(offers[1].Buckets) != 60 {
		t.Fatalf("the empty range should travel with the offer it fits in: %+v", offers[1].Ranges)
	}
}

func TestChunkOffersSplitsARangeThatDoesNotFit(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ranges := []TimeRange{{From: start, To: start.Add(250 * 24 * time.Hour)}}
	buckets := []database.DigestBucket{}
	for i := 0; i < 250; i++ {
		buckets = append(buckets, database.DigestBucket{Start: start.Add(time.Duration(i) * 24 * time.Hour), Rows: 1})
	}

	offers := chunkOffers(ranges, buckets, 24*time.Hour, 100)

	if len(offers) != 3 {
		t.Fatalf("expected 3 offers (100, 100, 50 buckets), got %d", len(offers))
	}
	seen := 0
	previousTo := start
	for i, o := range offers {
		if len(o.Buckets) > 100 || len(o.Ranges) != 1 {
			t.Fatalf("offer %d has %d buckets in %d ranges", i, len(o.Buckets), len(o.Ranges))
		}
		if !o.Ranges[0].From.Equal(previousTo) {
			t.Fatalf("offer %d does not start where the previous one ended: %v vs %v", i, o.Ranges[0].From, previousTo)
		}
		if !o.Buckets[0].Start.Equal(o.Ranges[0].From) || !o.Buckets[len(o.Buckets)-1].Start.Before(o.Ranges[0].To) {
			t.Fatalf("offer %d carries buckets outside its range", i)
		}
		previousTo = o.Ranges[0].To
		seen += len(o.Buckets)
	}
	if seen != 250 || !previousTo.Equal(ranges[0].To) {
		t.Fatalf("splitting lost something: %d buckets, ends at %v", seen, previousTo)
	}
}
