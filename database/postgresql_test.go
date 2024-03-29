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

	f := false
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

			mappedSelectQuery := `SELECT "time", "connector", "type", "context", "path", "value", "uuid", "origin" FROM "mapped_data" WHERE "uuid" = $1`
			var written *message.Mapped
			rows, err := db.GetConnection().Query(context.Background(), mappedSelectQuery, input.Updates[0].Source.Uuid)
			Expect(err).ShouldNot(HaveOccurred())
			defer rows.Close()
			rowCount := 0

			for rows.Next() {
				rowCount++

				written = message.NewMapped().AddUpdate(message.NewUpdate().AddValue(message.NewValue()))
				rows.Scan(
					&written.Updates[0].Timestamp,
					&written.Updates[0].Source.Label,
					&written.Updates[0].Source.Type,
					&written.Context,
					&written.Updates[0].Values[0].Path,
					&written.Updates[0].Values[0].Value,
					&written.Updates[0].Source.Uuid,
					&written.Origin,
				)
			}
			Expect(rowCount).To(Equal(1))

			written.Updates[0].Values[0].Value, err = message.Decode(written.Updates[0].Values[0].Value)
			Expect(err).ShouldNot(HaveOccurred())
			writtenJSON, _ := json.Marshal(written)
			expectedJSON, _ := json.Marshal(expected)
			Expect(writtenJSON).To(Equal(expectedJSON))
		},
		Entry(
			"Mapped with alarm value",
			mappedNotificationValue,
			mappedNotificationValue,
		),
		Entry(
			"Mapped with string value",
			mappedStringValue,
			mappedStringValue,
		),
	)
})
