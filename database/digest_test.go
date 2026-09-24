package database_test

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	. "github.com/munnik/gosk/database"
	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The summaries a vessel and the central server compare while seeding an
// outbox with history, see transfer/outbox_digest.go.
//
// Two origins stand in for the two databases. The digest is computed over
// uuids and row counts and never over the origin, so the same set of
// source messages under two origins must summarise identically - which is
// the property the whole scheme rests on, and the one these specs are
// mostly about.
var _ = Describe("Uuid digest", Ordered, func() {
	const (
		vessel = "digestVessel"
		cloud  = "digestCloud"
	)

	// A fixed instant, so the buckets these uuids fall into are the same
	// on every run and a spec can name a prefix without guessing.
	base := time.Date(2026, 3, 14, 9, 26, 53, 0, time.UTC)

	// bounds of the whole uuid space, for a summary over everything.
	whole := func() (uuid.UUID, uuid.UUID) {
		lo, err := uuid.FromBytes(make([]byte, 16))
		Expect(err).ToNot(HaveOccurred())
		raw, err := hex.DecodeString(strings.Repeat("f", 32))
		Expect(err).ToNot(HaveOccurred())
		hi, err := uuid.FromBytes(raw)
		Expect(err).ToNot(HaveOccurred())
		return lo, hi
	}

	// write puts one source message with the given paths under an origin.
	write := func(origin string, u uuid.UUID, when time.Time, paths ...string) {
		s := message.NewSource().WithLabel("digestConnector").WithType("json").WithUuid(u)
		update := message.NewUpdate().WithSource(*s).WithTimestamp(when)
		for i, p := range paths {
			update.AddValue(message.NewValue().WithPath(p).WithValue(float64(i) + 0.5))
		}
		db.WriteMapped(message.NewMapped().WithOrigin(origin).WithContext(origin).AddUpdate(update))
	}

	// uuids at one second intervals from base, so they spread over several
	// buckets at a long enough prefix while staying in one at a short one.
	var sources []uuid.UUID
	BeforeAll(func() {
		for i := range 40 {
			sources = append(sources, uuid.Must(uuid.NewV7AtTimePrecise(base.Add(time.Duration(i)*time.Second))))
		}
	})

	AfterEach(func() {
		_, err := db.GetConnection().Exec(
			context.Background(),
			`TRUNCATE "transfer_outbox", "mapped_data_matching_context", "mapped_data_other_context"`,
		)
		Expect(err).ShouldNot(HaveOccurred())
	})

	// both writes the same source messages under both origins.
	both := func(from, to int) {
		for i := from; i < to; i++ {
			write(vessel, sources[i], base.Add(time.Duration(i)*time.Second), "digest.a", "digest.b")
			write(cloud, sources[i], base.Add(time.Duration(i)*time.Second), "digest.a", "digest.b")
		}
		Expect(db.Flush()).To(Succeed())
	}

	digest := func(origin string, prefixLength int) []UuidBucket {
		lo, hi := whole()
		buckets, err := db.SelectUuidDigest(origin, lo, hi, prefixLength)
		Expect(err).ToNot(HaveOccurred())
		return buckets
	}

	It("summarises identical history identically", func() {
		both(0, 40)

		// Nine hex digits leaves 12 bits of the millisecond field below
		// them, so a bucket spans about four seconds and forty one-second
		// messages land in several of them.
		Expect(digest(vessel, 9)).To(Equal(digest(cloud, 9)), "the same source messages must summarise the same on both sides")
		Expect(len(digest(vessel, 9))).To(BeNumerically(">", 1), "these should not all fall in one bucket")
	})

	It("summarises the whole of history in one bucket at a short prefix", func() {
		both(0, 40)

		buckets := digest(vessel, 5)
		Expect(buckets).To(HaveLen(1), "forty seconds is well inside one three-day bucket")
		Expect(buckets[0].Uuids).To(BeEquivalentTo(40), "one per source message, not one per row")
	})

	It("differs in exactly one bucket when one source message is missing", func() {
		both(0, 40)
		// The cloud loses one, as a lost publish would have lost it.
		_, err := db.GetConnection().Exec(
			context.Background(),
			`DELETE FROM "mapped_data_matching_context" WHERE "origin" = $1 AND "uuid" = $2`,
			cloud, [16]byte(sources[17]),
		)
		Expect(err).ToNot(HaveOccurred())

		ours, theirs := digest(vessel, 9), digest(cloud, 9)
		Expect(ours).ToNot(Equal(theirs))

		byPrefix := map[string]UuidBucket{}
		for _, b := range theirs {
			byPrefix[b.Prefix] = b
		}
		differing := []string{}
		for _, b := range ours {
			if other, ok := byPrefix[b.Prefix]; !ok || other != b {
				differing = append(differing, b.Prefix)
			}
		}
		Expect(differing).To(HaveLen(1), "one missing source message must implicate one bucket and no others")
		// Without the dashes: a bucket prefix is a run of hex digits, and
		// a uuid's usual spelling has a dash in the middle of the ninth.
		Expect(strings.ReplaceAll(sources[17].String(), "-", "")).To(HavePrefix(differing[0]), "and it must be the bucket that message falls in")
	})

	It("notices a source message that arrived with only some of its rows", func() {
		both(0, 40)
		// Half a source message: the uuid is there, one of its two paths
		// is not. A count of source messages alone cannot see this, which
		// is why the digest hashes the row count with the uuid.
		_, err := db.GetConnection().Exec(
			context.Background(),
			`DELETE FROM "mapped_data_matching_context" WHERE "origin" = $1 AND "uuid" = $2 AND "path" = 'digest.b'`,
			cloud, [16]byte(sources[3]),
		)
		Expect(err).ToNot(HaveOccurred())

		ours, theirs := digest(vessel, 9), digest(cloud, 9)

		byPrefix := map[string]UuidBucket{}
		for _, b := range theirs {
			byPrefix[b.Prefix] = b
		}
		var found bool
		for _, b := range ours {
			other, ok := byPrefix[b.Prefix]
			if !ok {
				continue
			}
			if strings.HasPrefix(strings.ReplaceAll(sources[3].String(), "-", ""), b.Prefix) {
				found = true
				Expect(other.Uuids).To(Equal(b.Uuids), "the source message is still there, so the count is unchanged")
				Expect(other.Digest).ToNot(Equal(b.Digest), "but a missing row has to change the digest")
			}
		}
		Expect(found).To(BeTrue())
	})

	It("does not care what order things were written in", func() {
		// The digest is an exclusive or, so it describes a set. Writing
		// the same set backwards must produce the same numbers.
		for i := range 20 {
			write(vessel, sources[i], base.Add(time.Duration(i)*time.Second), "digest.a")
		}
		for i := 19; i >= 0; i-- {
			write(cloud, sources[i], base.Add(time.Duration(i)*time.Second), "digest.a")
		}
		Expect(db.Flush()).To(Succeed())

		Expect(digest(vessel, 9)).To(Equal(digest(cloud, 9)))
	})

	It("summarises only what falls inside the bounds it is given", func() {
		both(0, 40)

		// The bucket the twenty-first message falls in, at nine digits.
		prefix := strings.ReplaceAll(sources[20].String(), "-", "")[:9]
		lo, err := uuid.FromBytes(mustHex(prefix + strings.Repeat("0", 32-len(prefix))))
		Expect(err).ToNot(HaveOccurred())
		hi, err := uuid.FromBytes(mustHex(prefix + strings.Repeat("f", 32-len(prefix))))
		Expect(err).ToNot(HaveOccurred())

		buckets, err := db.SelectUuidDigest(vessel, lo, hi, 9)
		Expect(err).ToNot(HaveOccurred())
		Expect(buckets).To(HaveLen(1))
		Expect(buckets[0].Prefix).To(Equal(prefix))

		total := int64(0)
		for _, b := range digest(vessel, 9) {
			total += b.Uuids
		}
		Expect(buckets[0].Uuids).To(BeNumerically("<", total), "a bounded summary must not cover everything")
	})

	It("names the source messages in a bucket, with how many rows each has", func() {
		both(0, 5)
		lo, hi := whole()

		leaves, err := db.SelectUuidsInPrefix(vessel, lo, hi)
		Expect(err).ToNot(HaveOccurred())
		Expect(leaves).To(HaveLen(5))
		for _, l := range leaves {
			Expect(l.Rows).To(BeEquivalentTo(2), "two paths were written per source message")
		}
		Expect(leaves[0].Uuid).To(Equal(sources[0]), "oldest first, by uuid alone")
	})

	It("enqueues history into the outbox, and can be run again", func() {
		both(0, 5)

		enqueued, err := db.EnqueueOutbox(vessel, sources[:3])
		Expect(err).ToNot(HaveOccurred())
		Expect(enqueued).To(BeEquivalentTo(3))

		// A seeding run that is interrupted and started again must not
		// double anything up, and must not be an error.
		enqueued, err = db.EnqueueOutbox(vessel, sources[:3])
		Expect(err).ToNot(HaveOccurred())
		Expect(enqueued).To(BeEquivalentTo(0))

		entries, err := db.SelectOutbox(100)
		Expect(err).ToNot(HaveOccurred())
		Expect(entries).To(HaveLen(3))
		Expect(entries[0].Uuid).To(Equal(sources[0]), "seeded work is ordered like any other, oldest first")
	})

	It("summarises an origin it holds nothing for as nothing", func() {
		both(0, 5)
		Expect(digest("digestNobody", 9)).To(BeEmpty())
	})
})

func mustHex(s string) []byte {
	raw, err := hex.DecodeString(s)
	Expect(err).ToNot(HaveOccurred())
	return raw
}
