package database_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Mapped diagnostics", func() {
	write := func(context string, values ...*message.Value) uuid.UUID {
		id := uuid.New()
		s := message.NewSource().WithLabel("connectAmpero").WithType("modbus").WithUuid(id)
		u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Now().UTC().Truncate(time.Microsecond))
		for _, v := range values {
			u.AddValue(v)
		}
		db.WriteMapped(message.NewMapped().WithContext(context).WithOrigin("vessels.urn:mrn:imo:mmsi:244000001").AddUpdate(u))
		return id
	}
	readBack := func(id uuid.UUID, want int) map[string]message.SingleValueMapped {
		result := make(map[string]message.SingleValueMapped)
		Eventually(func() (int, error) {
			mapped, err := db.ReadMapped(`WHERE "uuid" = $1`, id)
			if err != nil {
				return 0, err
			}
			for _, m := range mapped {
				for _, svm := range m.ToSingleValueMapped() {
					result[svm.Path] = svm
				}
			}
			return len(result), nil
		}, "3s", "10ms").Should(Equal(want))
		return result
	}

	It("stores them with the value, and NULL when there are none", func() {
		id := write("vessels.urn:mrn:imo:mmsi:244000001",
			message.NewValue().WithPath("tanks.fuel.aft.currentLevel").WithValue(0.5).WithDiagnostics(map[string]any{"current": 0.012, "connected": true}),
			message.NewValue().WithPath("tanks.fuel.forward.currentLevel").WithValue(nil).WithDiagnostics(map[string]any{message.DiagnosticRejected: "all zeros"}),
			message.NewValue().WithPath("navigation.speedOverGround").WithValue(3.2),
			message.NewValue().WithPath("navigation.courseOverGroundTrue").WithValue(1.1).WithDiagnostics(map[string]any{}),
		)
		got := readBack(id, 4)

		Expect(got["tanks.fuel.aft.currentLevel"].Value).To(Equal(0.5))
		Expect(got["tanks.fuel.aft.currentLevel"].Diagnostics).To(Equal(map[string]any{"current": 0.012, "connected": true}))
		Expect(got["tanks.fuel.forward.currentLevel"].Value).To(BeNil())
		Expect(got["tanks.fuel.forward.currentLevel"].Diagnostics).To(Equal(map[string]any{message.DiagnosticRejected: "all zeros"}))
		Expect(got["navigation.speedOverGround"].Diagnostics).To(BeNil())

		var sqlNulls int
		Expect(db.GetConnection().QueryRow(context.Background(),
			`SELECT COUNT(*) FROM "mapped_data" WHERE "uuid" = $1 AND "diagnostics" IS NULL`, id).Scan(&sqlNulls)).To(Succeed())
		Expect(sqlNulls).To(Equal(2), "a value without diagnostics, or with an empty set, must store SQL NULL - not the jsonb null literal or {}")
	})

	It("stores them for another vessel's data too", func() {
		id := write("vessels.urn:mrn:imo:mmsi:244999999",
			message.NewValue().WithPath("navigation.position.raw").WithValue(1.0).WithDiagnostics(map[string]any{"sentence": "VDM"}),
		)
		Expect(readBack(id, 1)["navigation.position.raw"].Diagnostics).To(Equal(map[string]any{"sentence": "VDM"}))
	})

	It("replaces them along with the value when the same reading is written again", func() {
		id := uuid.New()
		ts := time.Now().UTC().Truncate(time.Microsecond)
		writeOnce := func(d map[string]any) {
			s := message.NewSource().WithLabel("connectAmpero").WithType("modbus").WithUuid(id)
			u := message.NewUpdate().WithSource(*s).WithTimestamp(ts).AddValue(message.NewValue().WithPath("a.b").WithValue(1.0).WithDiagnostics(d))
			db.WriteMapped(message.NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:244000001").WithOrigin("vessels.urn:mrn:imo:mmsi:244000001").AddUpdate(u))
		}
		writeOnce(map[string]any{"current": 0.004})
		Expect(readBack(id, 1)["a.b"].Diagnostics).To(Equal(map[string]any{"current": 0.004}))
		writeOnce(map[string]any{"current": 0.02})
		Eventually(func() (any, error) {
			mapped, err := db.ReadMapped(`WHERE "uuid" = $1`, id)
			if err != nil || len(mapped) != 1 {
				return nil, err
			}
			return mapped[0].ToSingleValueMapped()[0].Diagnostics["current"], nil
		}, "3s", "10ms").Should(Equal(0.02))
	})

	It("reads them back as the most recent value", func() {
		id := write("vessels.urn:mrn:imo:mmsi:244000001",
			message.NewValue().WithPath("tanks.fuel.port.currentLevel").WithValue(nil).WithDiagnostics(map[string]any{message.DiagnosticRejected: "error marker 0x8000 in register 52"}),
		)
		readBack(id, 1)
		recent, err := db.ReadMostRecentMapped(time.Now().Add(-time.Minute))
		Expect(err).NotTo(HaveOccurred())
		found := false
		for _, m := range recent {
			for _, svm := range m.ToSingleValueMapped() {
				if svm.Path == "tanks.fuel.port.currentLevel" {
					found = true
					Expect(svm.Diagnostics[message.DiagnosticRejected]).To(Equal("error marker 0x8000 in register 52"))
				}
			}
		}
		Expect(found).To(BeTrue())
	})
})
