package message_test

import (
	"github.com/google/uuid"
	. "github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Value", func() {
	DescribeTable(
		"Equals",
		func(left *Value, right *Value, expected bool) {
			Expect(left.Equals(*right)).To(Equal(expected))
		},
		Entry("with ints",
			NewValue().WithValue(42).WithPath("testpath").WithUuid(uuid.New()),
			NewValue().WithValue(42).WithPath("testpath").WithUuid(uuid.New()),
			true,
		),
		Entry("with bools",
			NewValue().WithValue(false).WithPath("testpath").WithUuid(uuid.New()),
			NewValue().WithValue(false).WithPath("testpath").WithUuid(uuid.New()),
			true,
		),
		Entry("with alarm and int",
			NewValue().WithValue(Alarm{State: false}).WithPath("testpath").WithUuid(uuid.New()),
			NewValue().WithValue(42).WithPath("testpath").WithUuid(uuid.New()),
			false,
		),
		Entry("with alarms",
			NewValue().WithValue(Alarm{State: false}).WithPath("testpath").WithUuid(uuid.New()),
			NewValue().WithValue(Alarm{State: false}).WithPath("testpath").WithUuid(uuid.New()),
			true,
		),
		Entry("with different alarms",
			NewValue().WithValue(Alarm{State: false}).WithPath("testpath").WithUuid(uuid.New()),
			NewValue().WithValue(Alarm{State: true}).WithPath("testpath").WithUuid(uuid.New()),
			false,
		),
		Entry("with different paths",
			NewValue().WithValue(false).WithPath("testpath").WithUuid(uuid.New()),
			NewValue().WithValue(false).WithPath("testpathpath").WithUuid(uuid.New()),
			false,
		),
	)
})
