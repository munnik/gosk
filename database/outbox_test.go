package database_test

import (
	"context"
	"time"

	. "github.com/munnik/gosk/database"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Outbox", Ordered, func() {
	const origin = "vessels.urn:mrn:imo:mmsi:244000001"
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	mapped := func(at time.Time, path string, value any) *message.Mapped {
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType")
		u := message.NewUpdate().WithSource(*s).WithTimestamp(at).AddValue(message.NewValue().WithPath(path).WithValue(value))
		return message.NewMapped().WithOrigin(origin).WithContext(origin).AddUpdate(u)
	}
	write := func(at time.Time, path string) {
		db.WriteMapped(mapped(at, path, 1.0))
	}
	pending := func() []OutboxEntry {
		entries, err := db.SelectPendingOutbox(100)
		Expect(err).ShouldNot(HaveOccurred())
		return entries
	}
	countMapped := func(context_, path string) func() (int, error) {
		return func() (int, error) {
			var count int
			err := db.GetConnection().QueryRow(
				context.Background(),
				`SELECT count(*) FROM "mapped_data" WHERE "context" = $1 AND "path" = $2`,
				context_, path,
			).Scan(&count)
			return count, err
		}
	}
	exec := func(sql string, args ...any) {
		_, err := db.GetConnection().Exec(context.Background(), sql, args...)
		Expect(err).ShouldNot(HaveOccurred())
	}

	BeforeAll(func() { db.SetOutbox(true) })
	AfterAll(func() { db.SetOutbox(false) })
	AfterEach(func() {
		exec(`TRUNCATE "transfer_outbox", "transfer_received", "mapped_data_matching_context", "mapped_data_other_context", "static_data"`)
	})

	It("marks the bucket of every row written, with the newest row time", func() {
		write(base.Add(5*time.Second), "a")
		write(base.Add(30*time.Second), "b")

		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		entry := pending()[0]
		Expect(entry.Origin).To(Equal(origin))
		Expect(entry.Bucket).To(BeTemporally("==", base))
		Expect(entry.Latest).To(BeTemporally("==", base.Add(30*time.Second)))
		Expect(entry.AckedThrough.IsZero()).To(BeTrue())
	})

	It("sends what is newer than the acknowledgement, and settles on it", func() {
		for i, path := range []string{"a", "b", "c"} {
			write(base.Add(time.Duration(i)*time.Second), path)
		}
		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		entry := pending()[0]

		rows, through, err := db.ReadOutboxDelivery(entry, 100)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rows).To(HaveLen(3))
		Expect(through).To(BeTemporally("==", base.Add(2*time.Second)))

		applied, err := db.AckOutbox(entry.Origin, entry.Bucket, entry.Generation, through)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(applied).To(BeTrue())
		Expect(pending()).To(BeEmpty())

		// Rows appended after the acknowledgement make the bucket pending
		// again, and only they are sent.
		write(base.Add(10*time.Second), "d")
		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		entry = pending()[0]
		Expect(entry.AckedThrough).To(BeTemporally("==", base.Add(2*time.Second)))

		rows, through, err = db.ReadOutboxDelivery(entry, 100)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rows).To(HaveLen(1))
		Expect(rows[0].Path).To(Equal("d"))
		Expect(through).To(BeTemporally("==", base.Add(10*time.Second)))
	})

	It("starts a bucket over when a row arrives late, and ignores the old generation's acknowledgement", func() {
		write(base, "a")
		write(base.Add(2*time.Second), "b")
		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		before := pending()[0]

		_, through, err := db.ReadOutboxDelivery(before, 100)
		Expect(err).ShouldNot(HaveOccurred())

		// Older than what was just read: cannot be expressed as "newer
		// than the acknowledgement", so the bucket resets.
		write(base.Add(time.Second), "late")
		Eventually(func() int64 { return pending()[0].Generation }, "3s", "10ms").ShouldNot(Equal(before.Generation))
		after := pending()[0]
		Expect(after.AckedThrough.IsZero()).To(BeTrue())

		// The acknowledgement of the delivery that missed the late row
		// arrives now, and must not apply.
		applied, err := db.AckOutbox(before.Origin, before.Bucket, before.Generation, through)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(applied).To(BeFalse())
		Expect(pending()).To(HaveLen(1))

		rows, _, err := db.ReadOutboxDelivery(after, 100)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rows).To(HaveLen(3))
	})

	It("marks the bucket of a row written by the query-by-query retry", func() {
		// design.beam is static data: next to the mapped row, its value
		// is written into static_data's numeric beam column, and a string
		// there fails server side with SQLSTATE 22P02. The suite flushes
		// every query on its own, which would fail that one alone; held
		// back and flushed together, it fails the whole batch, which sends
		// every query of the flush through retryIndividually - the path
		// that writes a row and its mark in a transaction of their own -
		// where the static_data query is dropped and the two mapped rows
		// are written.
		db.SetBatchFlushLength(100)
		defer db.SetBatchFlushLength(0)
		db.WriteMapped(mapped(base.Add(time.Second), "design.beam", "wide"))
		write(base.Add(2*time.Second), "a")
		Expect(pending()).To(BeEmpty(), "nothing may have been flushed yet")
		db.Flush()

		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		entry := pending()[0]
		Expect(entry.Latest).To(BeTemporally("==", base.Add(2*time.Second)))

		rows, _, err := db.ReadOutboxDelivery(entry, 100)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rows).To(HaveLen(2))
		Expect(rows[0].Path).To(Equal("design.beam"))
		Expect(rows[1].Path).To(Equal("a"))

		// Proof that the batch did fail and the retry path was taken.
		var beams int
		Expect(db.GetConnection().QueryRow(context.Background(), `SELECT count(*) FROM "static_data" WHERE "beam" IS NOT NULL`).Scan(&beams)).To(Succeed())
		Expect(beams).To(BeZero())
	})

	It("requeues a flush that times out on a bucket the sender holds, mark and all", func() {
		write(base, "a")
		Eventually(pending, "3s", "10ms").Should(HaveLen(1))

		// What the sender does while it reads a delivery: lock the bucket.
		// The next flush's upsert of that bucket then waits on it, and
		// with the suite's batch length of 0 that flush runs inside the
		// write itself, so the write returns once it has timed out.
		tx := lockingTransaction()
		defer tx.Rollback(context.Background())
		_, err := tx.Exec(context.Background(), `SELECT 1 FROM "transfer_outbox" WHERE "origin" = $1 AND "bucket" = $2 FOR UPDATE`, origin, base)
		Expect(err).ShouldNot(HaveOccurred())

		db.SetTimeout(300 * time.Millisecond)
		defer db.SetTimeout(5 * time.Second)
		write(base.Add(time.Second), "b")
		timedOut := time.Now()

		// Not written, not lost: the row is in the queue, with its mark,
		// waiting for a flush that is held back.
		Expect(countMapped(origin, "b")()).To(BeZero())
		Expect(pending()[0].Latest).To(BeTemporally("==", base))

		Expect(tx.Rollback(context.Background())).To(Succeed())
		db.SetTimeout(5 * time.Second)
		db.Flush()
		Expect(time.Since(timedOut)).To(BeNumerically(">=", 900*time.Millisecond), "the retry should have been held back")

		Expect(countMapped(origin, "b")()).To(Equal(1))
		Expect(pending()[0].Latest).To(BeTemporally("==", base.Add(time.Second)))
	})

	It("stores a delivery unless a newer generation of its bucket was stored already", func() {
		name := func(n string) *message.Mapped {
			// VesselInfo under the empty path, which mappedInsert stores
			// as path "name" and also in static_data.name.
			s := message.NewSource().WithLabel("ais").WithType("nmea0183")
			u := message.NewUpdate().WithSource(*s).WithTimestamp(base.Add(time.Second)).AddValue(message.NewValue().WithPath("").WithValue(message.VesselInfo{Name: &n}))
			return message.NewMapped().WithOrigin(origin).WithContext(origin).AddUpdate(u)
		}
		deliver := func(generation int64, value float64, vesselName string) bool {
			stored, err := db.StoreDelivery(origin, base, generation, []*message.Mapped{mapped(base, "a", value), name(vesselName)})
			Expect(err).ShouldNot(HaveOccurred())
			return stored
		}
		valueOfA := func() float64 {
			var v float64
			Expect(db.GetConnection().QueryRow(context.Background(), `SELECT "value"::float FROM "mapped_data" WHERE "context" = $1 AND "path" = 'a'`, origin).Scan(&v)).To(Succeed())
			return v
		}
		staticName := func() string {
			var n string
			Expect(db.GetConnection().QueryRow(context.Background(), `SELECT "name" FROM "static_data" WHERE "context" = $1`, origin).Scan(&n)).To(Succeed())
			return n
		}

		Expect(deliver(3, 1, "Old")).To(BeTrue(), "an unseen bucket")
		Expect(deliver(3, 2, "Old")).To(BeTrue(), "the same generation again: a retry, or the next chunk")
		Expect(deliver(4, 3, "New")).To(BeTrue(), "a newer generation")
		Expect(valueOfA()).To(Equal(3.0))
		Expect(staticName()).To(Equal("New"))

		// The delivery of the generation that was replaced turns up after
		// all. Nothing of it may land - not the rows, and not the name
		// that goes to static_data beside them - and there is nothing in
		// memory about it: a receiver that was restarted in between, or
		// another one on the same database, decides the same way.
		Expect(deliver(3, 1, "Old")).To(BeFalse())
		db.Flush()
		Expect(valueOfA()).To(Equal(3.0))
		Expect(staticName()).To(Equal("New"))

		// Other buckets and origins are unaffected.
		other := "vessels.urn:mrn:imo:mmsi:244000002"
		stored, err := db.StoreDelivery(origin, base.Add(time.Minute), 1, []*message.Mapped{mapped(base.Add(time.Minute), "a", 9)})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(stored).To(BeTrue())
		otherRow := mapped(base, "a", 1)
		otherRow.Origin, otherRow.Context = other, other
		stored, err = db.StoreDelivery(other, base, 1, []*message.Mapped{otherRow})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(stored).To(BeTrue())

		// And none of it marked the outbox: these rows came through it.
		Expect(pending()).To(BeEmpty())
	})

	It("stores a delivery whose static data postgres rejects, without the static data", func() {
		// design.beam is a string here, which the mapped row's jsonb column
		// takes and static_data's numeric beam column does not. The vessel
		// stored the row and dropped the static update (see the retry
		// spec above); the far end has to do the same, or the delivery
		// would fail on every attempt and hold its bucket back for good.
		stored, err := db.StoreDelivery(origin, base, 1, []*message.Mapped{mapped(base, "design.beam", 7.0), mapped(base.Add(time.Second), "design.beam", "wide"), mapped(base.Add(2*time.Second), "a", 1.0)})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(stored).To(BeTrue())

		var rows int
		Expect(db.GetConnection().QueryRow(context.Background(), `SELECT count(*) FROM "mapped_data" WHERE "context" = $1`, origin).Scan(&rows)).To(Succeed())
		Expect(rows).To(Equal(3))
		var beam *float64
		Expect(db.GetConnection().QueryRow(context.Background(), `SELECT "beam" FROM "static_data" WHERE "context" = $1`, origin).Scan(&beam)).To(Succeed())
		Expect(beam).ToNot(BeNil(), "the numeric beam before it was accepted")
		Expect(*beam).To(Equal(7.0))
	})

	It("ignores an acknowledgement that does not advance the bucket", func() {
		write(base, "a")
		write(base.Add(2*time.Second), "b")
		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		entry := pending()[0]

		rows, through, err := db.ReadOutboxDelivery(entry, 100)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rows).To(HaveLen(2))

		applied, err := db.AckOutbox(entry.Origin, entry.Bucket, entry.Generation, through)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(applied).To(BeTrue())
		Expect(pending()).To(BeEmpty())

		// The acknowledgement of an earlier, shorter delivery of the same
		// generation delivered again (QoS 1 does that) must not pull the
		// watermark back and make the bucket pending again.
		applied, err = db.AckOutbox(entry.Origin, entry.Bucket, entry.Generation, base)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(applied).To(BeFalse())
		Expect(pending()).To(BeEmpty())

		// Nor does the same one twice.
		applied, err = db.AckOutbox(entry.Origin, entry.Bucket, entry.Generation, through)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(applied).To(BeFalse())
	})

	It("splits a large bucket at a whole timestamp", func() {
		for _, path := range []string{"a", "b", "c"} {
			write(base, path)
		}
		for _, path := range []string{"d", "e"} {
			write(base.Add(time.Second), path)
		}
		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		entry := pending()[0]

		// Four would cut through the rows at base+1s, so the delivery
		// stops after the three at base.
		rows, through, err := db.ReadOutboxDelivery(entry, 4)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rows).To(HaveLen(3))
		Expect(through).To(BeTemporally("==", base))

		applied, err := db.AckOutbox(entry.Origin, entry.Bucket, entry.Generation, through)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(applied).To(BeTrue())

		entry = pending()[0]
		rows, through, err = db.ReadOutboxDelivery(entry, 4)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rows).To(HaveLen(2))
		Expect(through).To(BeTemporally("==", base.Add(time.Second)))
	})

	It("reports an entry that is gone", func() {
		write(base, "a")
		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		entry := pending()[0]
		entry.Generation++

		_, _, err := db.ReadOutboxDelivery(entry, 100)
		Expect(err).To(MatchError(ErrOutboxEntryGone))
	})

	It("prunes buckets that are acknowledged in full and long closed", func() {
		write(base, "a")
		Eventually(pending, "3s", "10ms").Should(HaveLen(1))
		entry := pending()[0]
		_, through, err := db.ReadOutboxDelivery(entry, 100)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = db.AckOutbox(entry.Origin, entry.Bucket, entry.Generation, through)
		Expect(err).ShouldNot(HaveOccurred())

		pruned, err := db.PruneOutbox(time.Hour)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(pruned).To(BeNumerically("==", 1))
	})

	It("summarises history per bucket on both ends alike, and enqueues what a range holds", func() {
		// Stored as deliveries, as the far end does, so nothing is marked
		// in the outbox by this.
		for minute := 0; minute < 3; minute++ {
			bucket := base.Add(time.Duration(minute) * time.Minute)
			rows := []*message.Mapped{}
			for _, path := range []string{"a", "b"} {
				rows = append(rows, mapped(bucket.Add(time.Second), path, float64(minute)))
			}
			stored, err := db.StoreDelivery(origin, bucket, 1, rows)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(stored).To(BeTrue())
		}
		Expect(pending()).To(BeEmpty())

		vessel, err := db.SelectDigest(origin, base, base.Add(time.Hour), OutboxBucket)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(vessel).To(HaveLen(3))
		for i, b := range vessel {
			Expect(b.Start).To(BeTemporally("==", base.Add(time.Duration(i)*time.Minute)))
			Expect(b.Rows).To(BeNumerically("==", 2))
		}

		// The same rows read again give the same summary; a row gone
		// changes exactly the bucket it was in.
		again, err := db.SelectDigest(origin, base, base.Add(time.Hour), OutboxBucket)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(again).To(Equal(vessel))
		exec(`DELETE FROM "mapped_data_matching_context" WHERE "path" = 'b' AND "time" = $1`, base.Add(time.Minute+time.Second))
		cloud, err := db.SelectDigest(origin, base, base.Add(time.Hour), OutboxBucket)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cloud[0]).To(Equal(vessel[0]))
		Expect(cloud[1].Rows).To(BeNumerically("==", 1))
		Expect(cloud[1].Hash).ToNot(Equal(vessel[1].Hash))
		Expect(cloud[2]).To(Equal(vessel[2]))

		// A day at a time: one bucket holding everything.
		day, err := db.SelectDigest(origin, base.Truncate(24*time.Hour), base.Truncate(24*time.Hour).Add(24*time.Hour), 24*time.Hour)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(day).To(HaveLen(1))
		Expect(day[0].Rows).To(BeNumerically("==", 5))

		// Seeding: every minute in the range becomes pending from the start.
		enqueued, err := db.EnqueueOutboxRange(origin, base, base.Add(2*time.Minute))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(enqueued).To(BeNumerically("==", 2))
		entries := pending()
		Expect(entries).To(HaveLen(2))
		for _, e := range entries {
			Expect(e.AckedThrough.IsZero()).To(BeTrue())
		}

		earliest, ok, err := db.EarliestMapped(origin)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(ok).To(BeTrue())
		Expect(earliest).To(BeTemporally("==", base.Add(time.Second)))
	})
})
