package database_test

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("Test database", Ordered, func() {
	now := time.Now()

	nan := math.NaN()
	zero := 0.0

	f := "normal"
	m := "testingNotification"

	mappedStringValue := func() message.Mapped {
		v := message.NewValue().WithPath("testingPath").WithValue("testValue")
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType")
		u := message.NewUpdate().WithSource(*s).WithTimestamp(now).AddValue(v)
		u.Timestamp = u.Timestamp.Add(-time.Duration(u.Timestamp.Nanosecond())) // resolution of time in postgresql is lower
		return *message.NewMapped().WithOrigin("testingOrigin").WithContext("testingContext").AddUpdate(u)
	}()
	mappedNotificationValue := func() message.Mapped {
		v := message.NewValue().WithPath("testingPath").WithValue(message.Notification{State: &f, Message: &m})
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType")
		u := message.NewUpdate().WithSource(*s).WithTimestamp(now).AddValue(v)
		u.Timestamp = u.Timestamp.Add(-time.Duration(u.Timestamp.Nanosecond())) // resolution of time in postgresql is lower
		return *message.NewMapped().WithOrigin("testingOrigin").WithContext("testingContext").AddUpdate(u)
	}()
	// A nil value (e.g. a cleared notification, see mapper/notification.go)
	// must round-trip through postgres as a JSON null, not a SQL NULL - the
	// "value" column is NOT NULL, so a bare SQL NULL would fail every write
	// it's ever bundled with, forever.
	mappedClearedValue := func() message.Mapped {
		v := message.NewValue().WithPath("testingPath").WithValue(nil)
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType")
		u := message.NewUpdate().WithSource(*s).WithTimestamp(now).AddValue(v)
		u.Timestamp = u.Timestamp.Add(-time.Duration(u.Timestamp.Nanosecond())) // resolution of time in postgresql is lower
		return *message.NewMapped().WithOrigin("testingOrigin").WithContext("testingContext").AddUpdate(u)
	}()

	// A non-finite float (NaN, +/-Inf) has no JSON representation, so pgx
	// cannot encode it into the jsonb "value" column and fails client-side,
	// before the query reaches postgres. There is no SQLSTATE on such a
	// failure, so retryIndividually used to treat it as transient and
	// requeue the row forever - which both never wrote it and forced every
	// later flush to retry its whole batch one query at a time.
	mappedUnencodableValue := func(v any) message.Mapped {
		value := message.NewValue().WithPath("unencodablePath").WithValue(v)
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType")
		u := message.NewUpdate().WithSource(*s).WithTimestamp(now).AddValue(value)
		u.Timestamp = u.Timestamp.Add(-time.Duration(u.Timestamp.Nanosecond()))
		return *message.NewMapped().WithOrigin("testingOrigin").WithContext("testingContext").AddUpdate(u)
	}

	countRows := func(path string) func() (int, error) {
		return func() (int, error) {
			var count int
			err := db.GetConnection().QueryRow(
				context.Background(),
				`SELECT count(*) FROM "mapped_data" WHERE "path" = $1`,
				path,
			).Scan(&count)
			return count, err
		}
	}

	// Read the counter off the default gatherer by name. testutil.ToFloat64
	// is what transfer/respond_test.go uses, but it needs the collector
	// itself and this is an external test package, so the database's
	// unexported prometheus fields are out of reach.
	dropsTotal := func() float64 {
		families, err := prometheus.DefaultGatherer.Gather()
		Expect(err).ShouldNot(HaveOccurred())
		for _, family := range families {
			if family.GetName() != "gosk_psql_drops_total" {
				continue
			}
			var total float64
			for _, metric := range family.GetMetric() {
				total += metric.GetCounter().GetValue()
			}
			return total
		}
		return 0
	}

	// Clear what the previous spec wrote. This replaces a full downgrade
	// of all 46 migrations, which is what the suite used to do between
	// specs: the schema is migrated once for the suite (see
	// database_suite_test.go), so only the rows need resetting. mapped_data
	// is a view over the two context tables since
	// 20231024113638_split_mapped_data, so the tables underneath it are the
	// ones to truncate.
	AfterEach(func() {
		_, err := db.GetConnection().Exec(
			context.Background(),
			`TRUNCATE "mapped_data_matching_context", "mapped_data_other_context", "raw_data", "static_data"`,
		)
		Expect(err).ShouldNot(HaveOccurred())
	})

	Describe("Reconnect", func() {
		// Inside an It: as a bare Context body this ran while ginkgo was
		// still building the spec tree, so it opened a connection before
		// BeforeSuite had a server to connect to, and its assertion counted
		// for nothing.
		It("reconnects after the connection is closed", func() {
			db.GetConnection().Close()

			Expect(db.GetConnection().Ping(context.Background())).To(Succeed())
		})
	})

	Describe("Write raw", func() {
		countRaw := func() (int, error) {
			var count int
			err := db.GetConnection().QueryRow(context.Background(), `SELECT count(*) FROM "raw_data"`).Scan(&count)
			return count, err
		}

		It("requeues a COPY that times out", func() {
			// Lock the table so the COPY waits; the suite's batch length
			// of 0 runs that flush inside WriteRaw, so the call returns
			// when the flush has timed out - with the row back in the
			// queue, where it used to be discarded.
			tx := lockingTransaction()
			defer tx.Rollback(context.Background())
			_, err := tx.Exec(context.Background(), `LOCK TABLE "raw_data" IN ACCESS EXCLUSIVE MODE`)
			Expect(err).ShouldNot(HaveOccurred())

			db.SetTimeout(300 * time.Millisecond)
			defer db.SetTimeout(5 * time.Second)
			db.WriteRaw(message.NewRaw().WithConnector("testConnector").WithType("testType").WithValue([]byte{1, 2, 3}))
			timedOut := time.Now()

			Expect(tx.Rollback(context.Background())).To(Succeed())
			Expect(countRaw()).To(BeZero())

			db.SetTimeout(5 * time.Second)
			db.Flush()
			Expect(time.Since(timedOut)).To(BeNumerically(">=", 900*time.Millisecond), "the retry should have been held back")
			Expect(countRaw()).To(Equal(1))
		})
	})

	DescribeTable("Write mapped",
		func(input *message.Mapped, expected message.Mapped) {
			db.WriteMapped(input)

			// WriteMapped only queues the write - flushBatch runs it against
			// postgres asynchronously (on a background goroutine, once the
			// batch crosses batch_flush_length - see database_suite_test.go).
			// Poll instead of asserting immediately, or this races the flush.
			mappedSelectQuery := `SELECT "time", "connector", "type", "context", "path", "value", "origin" FROM "mapped_data" WHERE "context" = $1 AND "path" = $2`
			var written *message.Mapped
			var err error
			Eventually(func() (int, error) {
				rows, queryErr := db.GetConnection().Query(context.Background(), mappedSelectQuery, input.Context, input.Updates[0].Values[0].Path)
				if queryErr != nil {
					return 0, queryErr
				}
				defer rows.Close()

				rowCount := 0
				for rows.Next() {
					rowCount++

					written = message.NewMapped().AddUpdate(message.NewUpdate().AddValue(message.NewValue()))
					if scanErr := rows.Scan(
						&written.Updates[0].Timestamp,
						&written.Updates[0].Source.Label,
						&written.Updates[0].Source.Type,
						&written.Context,
						&written.Updates[0].Values[0].Path,
						&written.Updates[0].Values[0].Value,
						&written.Origin,
					); scanErr != nil {
						return 0, scanErr
					}
				}
				return rowCount, rows.Err()
			}, "3s", "10ms").Should(Equal(1))

			written.Updates[0].Values[0].Value, err = message.Decode(written.Updates[0].Values[0].Value)
			Expect(err).ShouldNot(HaveOccurred())
			writtenJSON, _ := json.Marshal(written)
			expectedJSON, _ := json.Marshal(expected)
			Expect(writtenJSON).To(Equal(expectedJSON))
		},
		Entry(
			"Mapped with notification value",
			&mappedNotificationValue,
			mappedNotificationValue,
		),
		Entry(
			"Mapped with string value",
			&mappedStringValue,
			mappedStringValue,
		),
		Entry(
			"Mapped with cleared (nil) value",
			&mappedClearedValue,
			mappedClearedValue,
		),
	)

	DescribeTable("Drop non-finite values",
		func(value any) {
			dropped := mappedUnencodableValue(value)
			healthy := mappedStringValue

			// The row is absent from the table whether it was dropped or
			// requeued forever, so the table alone cannot tell the two
			// apart - the counter is where the difference shows.
			before := dropsTotal()

			// Queue a healthy write behind the one that cannot be encoded,
			// to pin down that dropping the latter does not take the former
			// with it.
			db.WriteMapped(&dropped)
			db.WriteMapped(&healthy)

			Eventually(countRows(healthy.Updates[0].Values[0].Path), "3s", "10ms").Should(Equal(1))
			Expect(countRows(dropped.Updates[0].Values[0].Path)()).To(Equal(0))
			Expect(dropsTotal()).To(BeNumerically("==", before+1))
		},
		Entry("a bare NaN", math.NaN()),
		Entry("a bare +Inf", math.Inf(1)),
		Entry("a bare -Inf", math.Inf(-1)),
		// Nothing upstream catches this one: runExpr rejects an expression
		// whose result is a bare non-finite float, but not one sitting in a
		// field of the struct it decoded the result into.
		Entry("a NaN nested in a struct", message.Position{Latitude: &nan, Longitude: &zero}),
	)
})
