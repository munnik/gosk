package message_test

import (
	. "github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Value", func() {
	f := false
	t := true
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
			NewValue().WithValue(Alarm{State: &f}).WithPath("testpath"),
			NewValue().WithValue(42).WithPath("testpath"),
			false,
		),
		Entry("with alarms",
			NewValue().WithValue(Alarm{State: &f}).WithPath("testpath"),
			NewValue().WithValue(Alarm{State: &f}).WithPath("testpath"),
			true,
		),
		Entry("with different alarms",
			NewValue().WithValue(Alarm{State: &f}).WithPath("testpath"),
			NewValue().WithValue(Alarm{State: &t}).WithPath("testpath"),
			false,
		),
		Entry("with different paths",
			NewValue().WithValue(false).WithPath("testpath"),
			NewValue().WithValue(false).WithPath("testpathpath"),
			false,
		),
	)
})
