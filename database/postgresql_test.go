package database_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/database"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test database", Ordered, func() {
	c := config.NewPostgresqlConfig("postgresql_test.yaml")
	db := NewPostgresqlDatabase(c)

	now := time.Now()

	f := "normal"
	m := "testingNotification"

	mappedStringValue := func() message.Mapped {
		v := message.NewValue().WithPath("testingPath").WithValue("testValue")
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType").WithUuid(uuid.New())
		u := message.NewUpdate().WithSource(*s).WithTimestamp(now).AddValue(v)
		u.Timestamp = u.Timestamp.Add(-time.Duration(u.Timestamp.Nanosecond())) // resolution of time in postgresql is lower
		return *message.NewMapped().WithOrigin("testingOrigin").WithContext("testingContext").AddUpdate(u)
	}()
	mappedNotificationValue := func() message.Mapped {
		v := message.NewValue().WithPath("testingPath").WithValue(message.Notification{State: &f, Message: &m})
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType").WithUuid(uuid.New())
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
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType").WithUuid(uuid.New())
		u := message.NewUpdate().WithSource(*s).WithTimestamp(now).AddValue(v)
		u.Timestamp = u.Timestamp.Add(-time.Duration(u.Timestamp.Nanosecond())) // resolution of time in postgresql is lower
		return *message.NewMapped().WithOrigin("testingOrigin").WithContext("testingContext").AddUpdate(u)
	}()

	BeforeEach(func() {
		err := db.UpgradeDatabase()
		Expect(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
		err := db.DowngradeDatabase()
		Expect(err).ShouldNot(HaveOccurred())
	})

	Describe("Reconnect",
		func() {
			Context("ping", func() {
				db.GetConnection().Close()
				err := db.GetConnection().Ping(context.Background())

				Expect(err).ShouldNot(HaveOccurred())
			})
		},
	)

	DescribeTable("Write mapped",
		func(input *message.Mapped, expected message.Mapped) {
			db.WriteMapped(input)

			// WriteMapped only queues the write - flushBatch runs it against
			// postgres asynchronously (on a background goroutine, once the
			// batch crosses batch_flush_length - see postgresql_test.yaml).
			// Poll instead of asserting immediately, or this races the flush.
			mappedSelectQuery := `SELECT "time", "connector", "type", "context", "path", "value", "uuid", "origin" FROM "mapped_data" WHERE "uuid" = $1`
			var written *message.Mapped
			var err error
			Eventually(func() (int, error) {
				rows, queryErr := db.GetConnection().Query(context.Background(), mappedSelectQuery, input.Updates[0].Source.Uuid)
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
						&written.Updates[0].Source.Uuid,
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
})
