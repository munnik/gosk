package writer_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	. "github.com/munnik/gosk/writer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test database", Ordered, func() {
	c := config.NewPostgresqlConfig("postgresql_test.yaml")
	w := NewPostgresqlWriter(c)

	now := time.Now()

	mappedStringValue := func() *message.Mapped {
		v := message.NewValue().WithPath("testingPath").WithUuid(uuid.Must(uuid.NewUUID())).WithValue("testValue")
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType")
		u := message.NewUpdate().WithSource(s).WithTimestamp(now).AddValue(v)
		u.Timestamp = u.Timestamp.Add(-time.Duration(u.Timestamp.Nanosecond())) // resolution of time in postgresql is lower
		return message.NewMapped().WithOrigin("testingOrigin").WithContext("testingContext").AddUpdate(u)
	}()
	mappedAlarmValue := func() *message.Mapped {
		v := message.NewValue().WithPath("testingPath").WithUuid(uuid.Must(uuid.NewUUID())).WithValue(message.Alarm{State: false, Message: "testingAlarm"})
		s := message.NewSource().WithLabel("testingLabel").WithType("testingType")
		u := message.NewUpdate().WithSource(s).WithTimestamp(now).AddValue(v)
		u.Timestamp = u.Timestamp.Add(-time.Duration(u.Timestamp.Nanosecond())) // resolution of time in postgresql is lower
		return message.NewMapped().WithOrigin("testingOrigin").WithContext("testingContext").AddUpdate(u)
	}()

	Describe("Prepare", func() {
		Context("execute", func() {
			err := w.UpgradeDatabase()

			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Describe("Reconnect", func() {
		Context("ping", func() {
			w.GetConnection().Close()
			err := w.GetConnection().Ping(context.Background())

			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	DescribeTable(
		"Write mapped",
		func(input *message.Mapped, expected *message.Mapped) {
			w.WriteSingleMappedEntry(input)

			mappedSelectQuery := `SELECT "time", "collector", "type", "context", "path", "value", "uuid", "origin" FROM "mapped_data" WHERE "uuid" = $1`
			var written *message.Mapped
			rows, err := w.GetConnection().Query(context.Background(), mappedSelectQuery, input.Updates[0].Values[0].Uuid)
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
					&written.Updates[0].Values[0].Uuid,
					&written.Origin,
				)
			}
			written.Updates[0].Values[0].Value, err = message.Decode(written.Updates[0].Values[0].Value)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(rowCount).To(Equal(1))
			writtenJSON, _ := json.Marshal(written)
			expectedJSON, _ := json.Marshal(expected)
			Expect(writtenJSON).To(Equal(expectedJSON))
		},
		Entry(
			"Mapped with alarm value",
			mappedAlarmValue,
			mappedAlarmValue,
		),
		Entry(
			"Mapped with string value",
			mappedStringValue,
			mappedStringValue,
		),
	)

	Describe("Cleanup", func() {
		Context("execute", func() {
			err := w.UpgradeDatabase()

			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
