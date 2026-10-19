package transfer

import (
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/database"
)

// Seeding an outbox with history, the other half of the scheme in
// outbox.go.
//
// outbox.go delivers what is written from now on. It says nothing about
// what was written before the outbox existed, or while the far end was
// losing data for some other reason. A vessel switched on today starts
// with an empty outbox and a database full of data the far end may or may
// not already hold, and the two obvious answers are both bad: put
// everything in the outbox and re-send years of data over a link that is
// the whole problem, or put nothing in and leave the gaps where they are.
//
// So: find the gaps first, and enqueue only those.
//
// Neither end can afford to send the other its keys - that is the same
// years of traffic, only smaller. Instead both summarise, and only the
// summaries that disagree are opened up. A bucket is a span of time; per
// bucket each end computes how many rows it holds and a hash of which
// rows they are (database.SelectDigest). The exchange walks down through
// days, then hours, then the outbox's own minute buckets:
//
//	vessel: here are my days, with counts and hashes
//	cloud:  these days differ; of these I hold almost nothing, send them whole
//	vessel: here are the hours of the days that differ
//	cloud:  these hours differ
//	vessel: here are the minutes of those hours
//	cloud:  these minutes differ
//	vessel: (puts those minutes in the outbox, and nothing else)
//
// A bucket both ends agree on is one line in one message, whatever it
// contains, so what the walk costs is set by how much is missing and
// where, not by how much history there is. Where loss is clustered - an
// outage, a month offline - it is a handful of messages; where it is
// scattered thinly over everything, it degenerates to naming every minute,
// which is the floor for an exact answer with minute buckets.
//
// Nothing here needs a uuid. The buckets come from the time column, which
// is also what the hypertable is partitioned on, so both ends read a range
// of chunks rather than an index over everything.

// digestLevels are the widths the walk descends through. Each level's
// buckets are summarised in one message per SeedMaxBuckets; the last
// level is the outbox bucket, so that what the walk ends on is exactly
// what the sender sends.
var digestLevels = []time.Duration{24 * time.Hour, time.Hour, database.OutboxBucket}

// TimeRange is a half-open span [From, To).
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// DigestOffer is a vessel describing part of its history: the buckets of
// Width it holds within Ranges, with how many rows and a hash each.
//
// Ranges are carried so the far end summarises exactly the same spans; a
// bucket both sides hold nothing under appears in neither summary and
// would otherwise be invisible, and a bucket the far end holds that the
// vessel does not is not a gap.
type DigestOffer struct {
	// Exchange ties an offer to its reply. A seeding run has exactly one
	// offer outstanding at a time, so this only serves to recognise the
	// answer to a question that has since timed out and been asked again.
	Exchange uuid.UUID               `json:"exchange"`
	Origin   string                  `json:"origin"`
	Width    time.Duration           `json:"width"`
	Ranges   []TimeRange             `json:"ranges"`
	Buckets  []database.DigestBucket `json:"buckets"`
}

// DigestReply is the far end's answer.
type DigestReply struct {
	Exchange uuid.UUID `json:"exchange"`
	Origin   string    `json:"origin"`
	// Missing names offered buckets the far end holds so little of that
	// listing what it lacks would cost more than being sent the whole
	// bucket. The vessel enqueues everything in them without another
	// exchange.
	Missing []time.Time `json:"missing,omitempty"`
	// Differ names offered buckets the far end's summary did not match,
	// to be opened up at the next level - or, at the last level,
	// enqueued.
	Differ []time.Time `json:"differ,omitempty"`
}

// compareDigests is the far end's side of an offer: which of the offered
// buckets does what it holds not account for, and which of those is it
// missing so much of that it would rather just be sent them.
//
// A bucket offered but absent here is one the far end has nothing under
// at all, which is the most important case and the reason the comparison
// is driven by the offer rather than by an intersection. Buckets present
// here but not offered are ignored: that is the far end holding something
// the vessel does not - data is trimmed by retention at different times
// on the two sides - and not a gap.
//
// takeWholeBelow is the fraction of the offered rows the far end has to
// hold for the bucket to be opened up rather than taken whole. It is a
// trade of round trips and summary traffic against over-sending: a bucket
// the far end holds a tenth of will mostly be sent anyway, and walking
// its children first only adds messages. The threshold leans towards
// opening up, because over-sending costs bandwidth on the link that is
// the whole problem.
func compareDigests(offered []database.DigestBucket, mine []database.DigestBucket, takeWholeBelow float64) (missing []time.Time, differ []time.Time) {
	byStart := make(map[int64]database.DigestBucket, len(mine))
	for _, b := range mine {
		byStart[b.Start.UnixNano()] = b
	}

	missing, differ = []time.Time{}, []time.Time{}
	for _, theirs := range offered {
		ours, ok := byStart[theirs.Start.UnixNano()]
		if ok && ours.Rows == theirs.Rows && ours.Hash == theirs.Hash {
			continue
		}
		if theirs.Rows > 0 && float64(ours.Rows)/float64(theirs.Rows) < takeWholeBelow {
			missing = append(missing, theirs.Start)
			continue
		}
		differ = append(differ, theirs.Start)
	}
	return missing, differ
}

// nextLevel is the width below width in digestLevels, and false at the
// last level.
func nextLevel(width time.Duration) (time.Duration, bool) {
	for i, w := range digestLevels {
		if w == width && i+1 < len(digestLevels) {
			return digestLevels[i+1], true
		}
	}
	return 0, false
}

// alignRange widens a span outwards to whole buckets of width, so that
// the walk's buckets line up with the far end's whatever the span was.
func alignRange(r TimeRange, width time.Duration) TimeRange {
	from := r.From.Truncate(width)
	to := r.To.Truncate(width)
	if to.Before(r.To) {
		to = to.Add(width)
	}
	return TimeRange{From: from, To: to}
}

// chunkOffers splits ranges and the buckets found in them into offers of
// at most maxBuckets buckets each. A range is kept whole with its own
// buckets when it fits; one that does not is cut at bucket boundaries into
// ranges that do. A range with no buckets still goes out, on its own if
// need be: the far end has to know it was looked at.
func chunkOffers(ranges []TimeRange, buckets []database.DigestBucket, width time.Duration, maxBuckets int) []DigestOffer {
	offers := []DigestOffer{}
	current := DigestOffer{Width: width, Ranges: []TimeRange{}, Buckets: []database.DigestBucket{}}
	flush := func() {
		if len(current.Ranges) > 0 {
			offers = append(offers, current)
		}
		current = DigestOffer{Width: width, Ranges: []TimeRange{}, Buckets: []database.DigestBucket{}}
	}

	i := 0
	for _, r := range ranges {
		inRange := []database.DigestBucket{}
		for i < len(buckets) && buckets[i].Start.Before(r.To) {
			if !buckets[i].Start.Before(r.From) {
				inRange = append(inRange, buckets[i])
			}
			i++
		}
		// A range with more buckets than one offer may carry - the whole
		// of history at the first level - is cut at bucket boundaries
		// into ranges that fit, each in an offer of its own.
		if maxBuckets > 0 && len(inRange) > maxBuckets {
			flush()
			for len(inRange) > maxBuckets {
				part, rest := inRange[:maxBuckets], inRange[maxBuckets:]
				current.Ranges = append(current.Ranges, TimeRange{From: r.From, To: rest[0].Start})
				current.Buckets = append(current.Buckets, part...)
				flush()
				r.From = rest[0].Start
				inRange = rest
			}
		}
		if len(current.Buckets)+len(inRange) > maxBuckets && len(current.Ranges) > 0 {
			flush()
		}
		current.Ranges = append(current.Ranges, r)
		current.Buckets = append(current.Buckets, inRange...)
	}
	flush()
	return offers
}
