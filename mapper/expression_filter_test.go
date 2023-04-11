package mapper_test

import (
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DoMap expression filter update", func() {
	mapper, _ := NewExpressionFilter(
		config.NewExpressionMappingConfig("expression_filter_test.yaml"),
	)
	now := time.Now()
	DescribeTable("Messages",
		func(m *ExpressionFilter, input *message.Mapped, expected *message.Mapped, expectError bool) {
			result, err := m.DoMap(input)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expected))
			}
		},
		Entry("no filters apply",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue("223890"),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue("223890"),
				),
			),
			false,
		),
		Entry("filters apply",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.torque").WithValue("223890"),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("multiple updates, only one filter applies",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.torque").WithValue("8409.6"),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue("8409.6"),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue("8409.6"),
				),
			),
			false,
		),
	)
})
