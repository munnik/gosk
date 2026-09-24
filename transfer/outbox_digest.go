package transfer

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/munnik/gosk/database"
	"github.com/munnik/uuid/v5"
)

// Seeding an outbox with history, the other half of the scheme in
// outbox.go.
//
// outbox.go delivers what is written from now on: the write path records
// every source message in transfer_outbox and it stays there until the far
// end says it has it. That says nothing about what was written before the
// outbox existed. A vessel switched on today starts with an empty outbox
// and a database full of data the cloud may or may not already hold, and
// the two obvious answers are both bad: enqueue everything and re-send
// years of data over a link that is the whole problem, or enqueue nothing
// and leave the gaps where they are.
//
// So: find the gaps first, and enqueue only those.
//
// Neither end can afford to send the other its keys - that is the same
// years of traffic, only smaller. Instead both summarise, and only the
// summaries that disagree are opened up. A bucket is a leading hex prefix
// of the uuid; since every uuid is a version 7 uuid, that prefix is a
// timestamp, so a bucket is a span of time and the whole walk partitions
// history without reading a "time" column, a continuous aggregate or a
// five minute period - none of which the outbox has, by design.
//
// The exchange walks down:
//
//	vessel: here are my buckets 1a2b3, 1a2b4, ... with counts and digests
//	cloud:  1a2b4 differs
//	vessel: here are the children of 1a2b4
//	cloud:  1a2b4c differs
//	...
//	vessel: bucket 1a2b4c9f is small, here are its source messages
//	cloud:  I am missing these eleven
//	vessel: (enqueues those eleven, and nothing else)
//
// What it costs is therefore set by how much is missing and where, not by
// how much history there is. A bucket both ends agree on is one line in
// one message, whatever it contains; the traffic concentrates on the parts
// that differ. Only at the very end does anything ask for data, and then
// only for the source messages the far end actually named.
//
// The fully scattered case - every bucket differing a little - degenerates
// to naming every uuid, which is the floor for any exact answer: you
// cannot find scattered gaps without comparing scattered keys. Even then a
// uuid is sixteen bytes against a delta's hundreds, and the deltas that
// follow are only the missing ones.
const (
	// outboxDigestTopic carries a vessel's summary of part of its history.
	outboxDigestTopic = "outbox/digest/%s"
	// outboxDigestReplyTopic carries the far end's answer: which buckets
	// to open up, or which source messages it is missing.
	outboxDigestReplyTopic = "outbox/digest-reply/%s"
)

const (
	// rootPrefixLength is where the walk starts. Five hex digits leaves 28
	// bits of the version 7 millisecond field below them, so a root bucket
	// spans 2^28 ms, a little over three days: a decade of history is
	// about twelve hundred of them, which is one small message.
	//
	// Starting here rather than at one digit skips four levels that would
	// each have had a single populated child - the top of a version 7
	// timestamp does not vary over any history we hold.
	rootPrefixLength = 5

	// maxSplit is how many hex digits one step may descend. Two gives up
	// to 256 children in a single summary; more than that and a message
	// describing a mostly-empty subtree gets expensive.
	maxSplit = 2

	// uuidHexDigits is a uuid written without its dashes.
	uuidHexDigits = 32
)

// OutboxDigestOffer is a vessel describing one part of its history.
//
// Buckets and Leaves are alternatives: a bucket is opened up into children
// until it is small enough to name outright, and then it is named.
type OutboxDigestOffer struct {
	// Exchange ties an offer to its reply. A seeding run has exactly one
	// offer outstanding at a time, so this is only needed to recognise the
	// answer to a question that has since timed out and been asked again.
	Exchange uuid.UUID `json:"exchange"`
	Origin   string    `json:"origin"`
	// Prefix is the bucket being described, empty for the whole of
	// history. The far end needs it to know which range to summarise,
	// since a bucket both sides hold nothing under appears in neither
	// summary and would otherwise be invisible.
	Prefix string `json:"prefix"`
	// Buckets are the children of Prefix, each with a prefix one or two
	// digits longer than Prefix.
	Buckets []database.UuidBucket `json:"buckets,omitempty"`
	// Leaves names the source messages under Prefix outright, instead of
	// summarising them again.
	Leaves []database.UuidCount `json:"leaves,omitempty"`
}

// OutboxDigestReply is the far end's answer.
type OutboxDigestReply struct {
	Exchange uuid.UUID `json:"exchange"`
	Origin   string    `json:"origin"`
	// Descend names the offered buckets whose count or digest did not
	// match, including buckets the far end holds nothing at all under.
	Descend []string `json:"descend,omitempty"`
	// Missing answers a Leaves offer: the source messages the far end does
	// not have, or has only some of the rows of.
	Missing []uuid.UUID `json:"missing,omitempty"`
	// Complete is set when the far end matched everything offered, so the
	// vessel can log a run as having found nothing rather than as having
	// been cut short.
	Complete bool `json:"complete"`
}

// prefixBounds turns a hex prefix into the uuid range it covers.
//
// Postgres compares uuids as sixteen bytes, most significant first, which
// is the order their hex spelling sorts in - so every uuid whose spelling
// starts with prefix lies between these two, and nothing else does. The
// bounds are not themselves valid version 7 uuids (nothing forces the
// version or variant nibbles here) and do not need to be: they are
// compared, never stored.
func prefixBounds(prefix string) (lo uuid.UUID, hi uuid.UUID, err error) {
	if len(prefix) > uuidHexDigits {
		return uuid.Nil, uuid.Nil, fmt.Errorf("prefix %q is longer than a uuid", prefix)
	}
	if lo, err = parseHexUuid(prefix + strings.Repeat("0", uuidHexDigits-len(prefix))); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if hi, err = parseHexUuid(prefix + strings.Repeat("f", uuidHexDigits-len(prefix))); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return lo, hi, nil
}

func parseHexUuid(digits string) (uuid.UUID, error) {
	raw, err := hex.DecodeString(digits)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%q is not a uuid prefix: %w", digits, err)
	}
	return uuid.FromBytes(raw)
}

// splitLength is how long the children of prefix should be, given how many
// source messages are under it and how few a bucket has to hold before
// naming them outright beats summarising them again.
//
// It aims each child at about leafSize, so a dense bucket is split wider
// than a sparse one and the walk does not spend a round trip halving
// something it could have cut into sixteenths.
func splitLength(prefix string, uuids int64, leafSize int) int {
	if prefix == "" {
		return rootPrefixLength
	}
	split := 1
	if leafSize > 0 && uuids > int64(leafSize) {
		// How many hex digits it takes to cut uuids down to leafSize.
		split = int(math.Ceil(math.Log2(float64(uuids)/float64(leafSize)) / 4))
	}
	split = max(1, min(split, maxSplit))
	return min(len(prefix)+split, uuidHexDigits)
}

// compareBuckets is the far end's side of a Buckets offer: which of the
// offered buckets does what it holds not account for.
//
// mine is this database's summary of the same range at the same prefix
// length, so the two are directly comparable. A bucket offered but absent
// here is one the far end has nothing under at all, which is the most
// important case and the reason the comparison is driven by the offer
// rather than by an intersection.
//
// Buckets present here but not offered are ignored. That is the far end
// holding something the vessel does not, which is not a gap - data is
// deleted by retention at different times on the two sides, and treating
// that as a difference would send the vessel looking for rows it has no
// business having.
func compareBuckets(offered []database.UuidBucket, mine []database.UuidBucket) []string {
	byPrefix := make(map[string]database.UuidBucket, len(mine))
	for _, b := range mine {
		byPrefix[b.Prefix] = b
	}

	descend := []string{}
	for _, theirs := range offered {
		ours, ok := byPrefix[theirs.Prefix]
		if !ok || ours.Uuids != theirs.Uuids || ours.Digest != theirs.Digest {
			descend = append(descend, theirs.Prefix)
		}
	}
	return descend
}

// compareLeaves is the far end's side of a Leaves offer: which of the
// named source messages is it missing, wholly or in part.
//
// Fewer rows counts as missing. A source message becomes several mapped
// rows, and the old count protocol could deliver some of them and lose the
// rest; re-sending the whole source message is how that is repaired, and
// the far end's insert is an upsert, so the rows it already had are
// rewritten rather than duplicated.
func compareLeaves(offered []database.UuidCount, mine []database.UuidCount) []uuid.UUID {
	byUuid := make(map[uuid.UUID]int64, len(mine))
	for _, c := range mine {
		byUuid[c.Uuid] = c.Rows
	}

	missing := []uuid.UUID{}
	for _, theirs := range offered {
		if rows, ok := byUuid[theirs.Uuid]; !ok || rows < theirs.Rows {
			missing = append(missing, theirs.Uuid)
		}
	}
	return missing
}
