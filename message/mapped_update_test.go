package message_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Update", func() {
	u1 := NewUpdate().WithSource(*NewSource().WithLabel("testsource").WithType("test").WithUuid(uuid.New())).WithTimestamp(time.Now())
	u1.AddValue(NewValue().WithPath("testpath").WithValue(42))
	u2 := NewUpdate().WithSource(*NewSource().WithLabel("testsource").WithType("test").WithUuid(uuid.New())).WithTimestamp(time.Now())
	u2.AddValue(NewValue().WithPath("testpath").WithValue(42))
	u2.AddValue(NewValue().WithPath("testpathpath").WithValue(false))
	u3 := NewUpdate().WithSource(*NewSource().WithLabel("testsource").WithType("test").WithUuid(uuid.New())).WithTimestamp(time.Now())
	u3.AddValue(NewValue().WithPath("testpathpath").WithValue(false))
	u3.AddValue(NewValue().WithPath("testpath").WithValue(42))
	u4 := NewUpdate().WithSource(*NewSource().WithLabel("testsourcesource").WithType("test").WithUuid(uuid.New())).WithTimestamp(time.Now())
	u4.AddValue(NewValue().WithPath("testpathpath").WithValue(false))
	u4.AddValue(NewValue().WithPath("testpath").WithValue(42))
	DescribeTable(
		"Equals",
		func(left *Update, right *Update, expected bool) {
			Expect(left.Equals(*right)).To(Equal(expected))
		},
		Entry("with different values", u1, u2, false),
		Entry("with equal values", u2, u2, true),
		Entry("with equal in different order values", u2, u3, true),
		Entry("with different sources", u3, u4, true),
	)
})
