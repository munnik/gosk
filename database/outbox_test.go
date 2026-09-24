package database_test

import (
	"time"

	"context"

	. "github.com/munnik/gosk/database"
	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The outbox, see transfer/outbox.go. These check the half that lives in
// the database: that writing a row queues it, that a row which arrived
// through the transfer protocol does not queue itself back, that pending
// work comes out oldest first without any timestamp being consulted, and
// that acknowledging removes it.
var _ = Describe("Transfer outbox", Ordered, func() {
	// The suite's own handle, with the outbox switched on for these specs
	// and off again afterwards. A second PostgresqlDatabase cannot be
	// constructed in the same process: NewPostgresqlDatabase registers
	// Prometheus collectors under fixed names and promauto panics on a
	// duplicate.
	//
	// db is read inside each spec rather than copied into a variable here.
	// This function body runs while ginkgo is still building the spec
	// tree, before BeforeSuite has assigned it, so a copy taken here is
	// nil.
	BeforeAll(func() { db.SetOutbox(true) })
	AfterAll(func() { db.SetOutbox(false) })

	AfterEach(func() {
		_, err := db.GetConnection().Exec(
			context.Background(),
			`TRUNCATE "transfer_outbox", "mapped_data_matching_context", "mapped_data_other_context"`,
		)
		Expect(err).ShouldNot(HaveOccurred())
	})

	// at builds a mapped message whose uuid embeds its own timestamp, the
	// way the pipeline produces them.
	at := func(offset time.Duration, path string, transferUuid uuid.UUID) (*message.Mapped, uuid.UUID) {
		when := time.Now().Add(offset).Truncate(time.Microsecond)
		u := uuid.Must(uuid.NewV7AtTimePrecise(when))
		s := message.NewSource().WithLabel("outboxConnector").WithType("json").WithUuid(u)
		s.TransferUuid = transferUuid
		update := message.NewUpdate().WithSource(*s).WithTimestamp(when).AddValue(
			message.NewValue().WithPath(path).WithValue(1.0),
		)
		return message.NewMapped().WithOrigin("outboxOrigin").WithContext("outboxOrigin").AddUpdate(update), u
	}

	It("queues a source message when its rows are written", func() {
		m, u := at(-3*time.Minute, "outbox.one", uuid.Nil)
		db.WriteMapped(m)
		Expect(db.Flush()).To(Succeed())

		entries, err := db.SelectOutbox(100)
		Expect(err).ToNot(HaveOccurred())
		Expect(entries).To(ContainElement(OutboxEntry{Origin: "outboxOrigin", Uuid: u}))
	})

	It("queues one entry per source message, not one per row", func() {
		// Two paths under the same source message, as one raw message
		// mapping to several paths produces.
		when := time.Now().Add(-2 * time.Minute).Truncate(time.Microsecond)
		u := uuid.Must(uuid.NewV7AtTimePrecise(when))
		s := message.NewSource().WithLabel("outboxConnector").WithType("json").WithUuid(u)
		update := message.NewUpdate().WithSource(*s).WithTimestamp(when).
			AddValue(message.NewValue().WithPath("outbox.two.a").WithValue(1.0)).
			AddValue(message.NewValue().WithPath("outbox.two.b").WithValue(2.0))
		db.WriteMapped(message.NewMapped().WithOrigin("outboxOrigin").WithContext("outboxOrigin").AddUpdate(update))
		Expect(db.Flush()).To(Succeed())

		entries, err := db.SelectOutbox(1000)
		Expect(err).ToNot(HaveOccurred())

		matching := 0
		for _, e := range entries {
			if e.Uuid == u {
				matching++
			}
		}
		Expect(matching).To(Equal(1), "two rows of one source message should leave one outbox entry")
	})

	It("does not queue a row that arrived through the transfer protocol", func() {
		m, u := at(-time.Minute, "outbox.transferred", uuid.Must(uuid.NewV7Precise()))
		db.WriteMapped(m)
		Expect(db.Flush()).To(Succeed())

		entries, err := db.SelectOutbox(1000)
		Expect(err).ToNot(HaveOccurred())
		for _, e := range entries {
			Expect(e.Uuid).ToNot(Equal(u), "a row sent to us by someone else would be sent straight back")
		}
	})

	It("returns pending work oldest first, by uuid alone", func() {
		// Written newest first, so ordering cannot come from insertion
		// order; and nothing in SelectOutbox looks at a timestamp.
		newer, newerUuid := at(-10*time.Second, "outbox.order.newer", uuid.Nil)
		older, olderUuid := at(-20*time.Second, "outbox.order.older", uuid.Nil)
		db.WriteMapped(newer)
		Expect(db.Flush()).To(Succeed())
		db.WriteMapped(older)
		Expect(db.Flush()).To(Succeed())

		entries, err := db.SelectOutbox(1000)
		Expect(err).ToNot(HaveOccurred())

		olderAt, newerAt := -1, -1
		for i, e := range entries {
			switch e.Uuid {
			case olderUuid:
				olderAt = i
			case newerUuid:
				newerAt = i
			}
		}
		Expect(olderAt).ToNot(Equal(-1))
		Expect(newerAt).ToNot(Equal(-1))
		Expect(olderAt).To(BeNumerically("<", newerAt), "version 7 uuids sort by time, so the older one comes first")
	})

	It("reads back the rows of a source message without a time predicate", func() {
		m, u := at(-30*time.Second, "outbox.readback", uuid.Nil)
		db.WriteMapped(m)
		Expect(db.Flush()).To(Succeed())

		deltas, err := db.SelectMappedByUuids("outboxOrigin", []uuid.UUID{u})
		Expect(err).ToNot(HaveOccurred())
		Expect(deltas).ToNot(BeEmpty())
		for _, d := range deltas {
			for _, update := range d.Updates {
				Expect(update.Source.Uuid).To(Equal(u))
			}
		}
	})

	It("clears only what was acknowledged, and is safe to repeat", func() {
		keep, keepUuid := at(-15*time.Second, "outbox.keep", uuid.Nil)
		drop, dropUuid := at(-16*time.Second, "outbox.drop", uuid.Nil)
		db.WriteMapped(keep)
		db.WriteMapped(drop)
		Expect(db.Flush()).To(Succeed())

		removed, err := db.DeleteOutbox("outboxOrigin", []uuid.UUID{dropUuid})
		Expect(err).ToNot(HaveOccurred())
		Expect(removed).To(BeEquivalentTo(1))

		// Acknowledging the same thing twice must not be an error: a
		// duplicated ack is a normal consequence of QoS 1.
		removed, err = db.DeleteOutbox("outboxOrigin", []uuid.UUID{dropUuid})
		Expect(err).ToNot(HaveOccurred())
		Expect(removed).To(BeEquivalentTo(0))

		entries, err := db.SelectOutbox(1000)
		Expect(err).ToNot(HaveOccurred())
		var sawKeep, sawDrop bool
		for _, e := range entries {
			if e.Uuid == keepUuid {
				sawKeep = true
			}
			if e.Uuid == dropUuid {
				sawDrop = true
			}
		}
		Expect(sawKeep).To(BeTrue(), "an unacknowledged entry must stay pending")
		Expect(sawDrop).To(BeFalse(), "an acknowledged entry must be gone")
	})

	It("writes nothing to the outbox when it is not enabled", func() {
		db.SetOutbox(false)
		defer db.SetOutbox(true)

		m, u := at(-5*time.Second, "outbox.disabled", uuid.Nil)
		db.WriteMapped(m)
		Expect(db.Flush()).To(Succeed())

		entries, err := db.SelectOutbox(1000)
		Expect(err).ToNot(HaveOccurred())
		for _, e := range entries {
			Expect(e.Uuid).ToNot(Equal(u))
		}
	})
})
