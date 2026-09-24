package mapper_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// embeddedMillis reads the 48 bit millisecond timestamp out of a version 7
// UUID, the same arithmetic uuid_timestamp() performs server side.
func embeddedMillis(u uuid.UUID) int64 {
	return int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 |
		int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
}

var _ = Describe("DoMap json timestampExpression", func() {
	timestampMapper := func() *JSONMapper {
		m, err := NewJSONMapper(
			config.MapperConfig{Context: "testingContext"},
			[]config.JSONMappingConfig{{
				MappingConfig: config.MappingConfig{
					Expression:          "json['pwr']",
					TimestampExpression: "json['ts']",
					Path:                "propulsion.mainEngine.drive.power",
				},
			}},
		)
		Expect(err).ToNot(HaveOccurred())
		return m
	}

	// rawAt builds a raw message that arrived at arrived and whose payload
	// claims to have been measured at measured.
	rawAt := func(arrived time.Time, measured time.Time) *message.Raw {
		payload, err := json.Marshal(map[string]string{
			"pwr": "8409.6",
			"ts":  measured.Format(time.RFC3339),
		})
		Expect(err).ToNot(HaveOccurred())

		r := message.NewRaw().
			WithConnector("testingConnector").
			WithType(config.JSONType).
			WithValue(payload)
		r.Timestamp = arrived
		return r
	}

	firstUpdate := func(m *message.Mapped) message.Update {
		ExpectWithOffset(1, m.Updates).To(HaveLen(1))
		return m.Updates[0]
	}

	It("takes the timestamp from the payload, and derives a matching uuid", func() {
		arrived := time.Now().Truncate(time.Second)
		measured := arrived.Add(-90 * time.Second)

		raw := rawAt(arrived, measured)
		out, err := timestampMapper().DoMap(raw)
		Expect(err).ToNot(HaveOccurred())

		u := firstUpdate(out)
		Expect(u.Timestamp).To(BeTemporally("==", measured))

		// The whole point: the uuid's embedded time agrees with the row's
		// time, rather than saying when the bytes happened to arrive.
		Expect(u.Source.Uuid.Version()).To(BeEquivalentTo(7))
		Expect(embeddedMillis(u.Source.Uuid)).To(Equal(measured.UnixMilli()))
		Expect(u.Source.Uuid).ToNot(Equal(raw.Uuid))
	})

	It("keeps the raw uuid when the payload carries no timestamp of its own", func() {
		arrived := time.Now().Truncate(time.Second)

		raw := message.NewRaw().
			WithConnector("testingConnector").
			WithType(config.JSONType).
			WithValue([]byte(`{"pwr":"8409.6"}`))
		raw.Timestamp = arrived

		out, err := timestampMapper().DoMap(raw)
		Expect(err).ToNot(HaveOccurred())

		u := firstUpdate(out)
		Expect(u.Timestamp).To(BeTemporally("==", arrived))
		Expect(u.Source.Uuid).To(Equal(raw.Uuid))
	})

	DescribeTable("rejects a payload timestamp further than a year from arrival",
		func(offset time.Duration) {
			arrived := time.Now().Truncate(time.Second)
			raw := rawAt(arrived, arrived.Add(offset))

			out, err := timestampMapper().DoMap(raw)
			Expect(err).ToNot(HaveOccurred())

			u := firstUpdate(out)
			Expect(u.Timestamp).To(BeTemporally("==", arrived), "should have fallen back to the arrival time")
			Expect(u.Source.Uuid).To(Equal(raw.Uuid), "and kept the raw message's uuid")
		},
		// The future direction is the one the old check missed: it
		// compared arrival-minus-payload against +365 days, and a future
		// timestamp makes that difference negative, which is below any
		// positive threshold.
		Entry("two years in the future", 2*365*24*time.Hour),
		Entry("a century in the future", 100*365*24*time.Hour),
		Entry("two years in the past", -2*365*24*time.Hour),
	)

	DescribeTable("accepts a payload timestamp within a year of arrival",
		func(offset time.Duration) {
			arrived := time.Now().Truncate(time.Second)
			measured := arrived.Add(offset)
			raw := rawAt(arrived, measured)

			out, err := timestampMapper().DoMap(raw)
			Expect(err).ToNot(HaveOccurred())

			u := firstUpdate(out)
			Expect(u.Timestamp).To(BeTemporally("==", measured))
			Expect(embeddedMillis(u.Source.Uuid)).To(Equal(measured.UnixMilli()))
		},
		Entry("a minute behind", -time.Minute),
		Entry("a minute ahead", time.Minute),
		Entry("just inside a year behind", -364*24*time.Hour),
		Entry("just inside a year ahead", 364*24*time.Hour),
	)

	It("is deterministic, so re-mapping the same message gives the same uuid", func() {
		arrived := time.Now().Truncate(time.Second)
		raw := rawAt(arrived, arrived.Add(-time.Hour))

		first, err := timestampMapper().DoMap(raw)
		Expect(err).ToNot(HaveOccurred())
		second, err := timestampMapper().DoMap(raw)
		Expect(err).ToNot(HaveOccurred())

		Expect(firstUpdate(first).Source.Uuid).To(
			Equal(firstUpdate(second).Source.Uuid),
			fmt.Sprintf("raw uuid %s mapped twice", raw.Uuid),
		)
	})
})
