package message_test

import (
	"encoding/json"

	. "github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Value", func() {
	f := "normal"
	t := "alarm"
	DescribeTable(
		"Equals",
		func(left *Value, right *Value, expected bool) {
			Expect(left.Equals(*right)).To(Equal(expected))
		},
		Entry("with ints",
			NewValue().WithValue(42).WithPath("testpath"),
			NewValue().WithValue(42).WithPath("testpath"),
			true,
		),
		Entry("with bools",
			NewValue().WithValue(false).WithPath("testpath"),
			NewValue().WithValue(false).WithPath("testpath"),
			true,
		),
		Entry("with alarm and int",
			NewValue().WithValue(Notification{State: &f}).WithPath("testpath"),
			NewValue().WithValue(42).WithPath("testpath"),
			false,
		),
		Entry("with alarms",
			NewValue().WithValue(Notification{State: &f}).WithPath("testpath"),
			NewValue().WithValue(Notification{State: &f}).WithPath("testpath"),
			true,
		),
		Entry("with different alarms",
			NewValue().WithValue(Notification{State: &f}).WithPath("testpath"),
			NewValue().WithValue(Notification{State: &t}).WithPath("testpath"),
			false,
		),
		Entry("with different paths",
			NewValue().WithValue(false).WithPath("testpath"),
			NewValue().WithValue(false).WithPath("testpathpath"),
			false,
		),
	)
})

var _ = Describe("Decode", func() {
	It("returns nil for a nil input instead of matching the first all-optional struct it tries", func() {
		// mapstructure.DecodeMetadata trivially "succeeds" decoding a nil
		// input into any all-optional struct (e.g. Position), leaving it at
		// its zero value with no Unused fields to report, so a naive
		// implementation would mistake a nil for that struct's zero value.
		decoded, err := Decode(nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(decoded).To(BeNil())
	})

	It("keeps a JSON null value nil after unmarshalling, e.g. a cleared notification", func() {
		v := &Value{}
		err := json.Unmarshal([]byte(`{"path":"notifications.test","value":null}`), v)
		Expect(err).ToNot(HaveOccurred())
		Expect(v.Value).To(BeNil())
	})
})
