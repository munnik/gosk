package mapper_test

import (
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var sourceUuid = uuid.Must(uuid.NewV7Precise())

var _ = Describe("NewConnectorStatusUpdate", func() {
	It("raises an alarm notification when disconnected", func() {
		result := NewConnectorStatusUpdate("testingContext", "Ampero modules", false, sourceUuid)

		Expect(result.Context).To(Equal("testingContext"))
		Expect(result.Origin).To(Equal("testingContext"))
		Expect(result.Updates).To(HaveLen(1))
		Expect(result.Updates[0].Values).To(HaveLen(1))

		value := result.Updates[0].Values[0]
		Expect(value.Path).To(Equal("notifications.connectors.Amperomodules.connected"))

		notification, ok := value.Value.(message.Notification)
		Expect(ok).To(BeTrue())
		Expect(*notification.State).To(Equal("alarm"))
		Expect(*notification.Message).To(Equal("no data received from Ampero modules"))
	})

	It("clears the notification when connected", func() {
		result := NewConnectorStatusUpdate("testingContext", "Ampero modules", true, sourceUuid)

		Expect(result.Updates).To(HaveLen(1))
		Expect(result.Updates[0].Values).To(HaveLen(1))

		value := result.Updates[0].Values[0]
		Expect(value.Path).To(Equal("notifications.connectors.Amperomodules.connected"))
		Expect(value.Value).To(BeNil())
	})
})

var _ = Describe("MapConnectorStatus", func() {
	It("is implemented by ModbusMapper using its own context", func() {
		m, err := NewModbusMapper(config.MapperConfig{Context: "testingContext"}, nil)
		Expect(err).ToNot(HaveOccurred())

		result := m.MapConnectorStatus("Ampero modules", false, sourceUuid)
		Expect(result.Context).To(Equal("testingContext"))
		Expect(result.Updates).To(HaveLen(1))
		Expect(result.Updates[0].Values).To(HaveLen(1))
		Expect(result.Updates[0].Values[0].Path).To(Equal("notifications.connectors.Amperomodules.connected"))
	})
})

var _ = Describe("connector status notification uuids", func() {
	// These rows used to carry uuid.Nil, which left them with no identity
	// and, once mapped_data's time could in principle be read back out of
	// the uuid, no time either. See section 7.1.1 of TRANSFER_REVIEW.md.
	It("derives the uuid from the status report that caused it", func() {
		source := uuid.Must(uuid.NewV7Precise())
		result := NewConnectorStatusUpdate("testingContext", "Ampero modules", false, source)

		got := result.Updates[0].Source.Uuid
		Expect(got).ToNot(Equal(uuid.Nil))
		Expect(got.Version()).To(BeEquivalentTo(7))
		// the random half comes from the source, the timestamp half does not
		Expect(got[8:]).To(Equal(source[8:]))
		Expect(got[:8]).ToNot(Equal(source[:8]))
	})

	It("still produces a usable uuid when there is no source", func() {
		result := NewConnectorStatusUpdate("testingContext", "Ampero modules", false, uuid.Nil)

		got := result.Updates[0].Source.Uuid
		Expect(got).ToNot(Equal(uuid.Nil))
		Expect(got.Version()).To(BeEquivalentTo(7))
		Expect(got.Variant()).To(Equal(uuid.VariantRFC9562))
	})
})
