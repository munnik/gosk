package mapper_test

import (
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NewConnectorStatusUpdate", func() {
	It("raises an alarm notification when disconnected", func() {
		result := NewConnectorStatusUpdate("testingContext", "Ampero modules", false)

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
		result := NewConnectorStatusUpdate("testingContext", "Ampero modules", true)

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

		result := m.MapConnectorStatus("Ampero modules", false)
		Expect(result.Context).To(Equal("testingContext"))
		Expect(result.Updates).To(HaveLen(1))
		Expect(result.Updates[0].Values).To(HaveLen(1))
		Expect(result.Updates[0].Values[0].Path).To(Equal("notifications.connectors.Amperomodules.connected"))
	})
})
