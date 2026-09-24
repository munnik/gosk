package mapper_test

import (
	"time"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DoMap aggregate same update", func() {
	mapper, _ := NewAggregateMapper(
		config.MapperConfig{Context: "testingContext"},
		config.NewExpressionMappingConfig("aggregate_test.yaml"),
	)
	now := time.Now()
	DescribeTable("Messages",
		func(m *AggregateMapper, input *message.Mapped, expected *message.Mapped, expectError bool) {
			result, err := m.DoMap(input)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expected))
			}
		},
		Entry("no matching path",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
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
		Entry("single matching path",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port.drive.power").WithValue(5.6),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port.drive.power").WithValue(5.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("signalk").WithType(config.SignalKType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.plusfive.drive.power").WithValue(10.6),
				),
			),
			false,
		),
		Entry("two matching paths",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port2.drive.power").WithValue(5.6),
				).AddValue(
					message.NewValue().WithPath("propulsion.starboard2.drive.power").WithValue(5.6),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port2.drive.power").WithValue(5.6),
				).AddValue(
					message.NewValue().WithPath("propulsion.starboard2.drive.power").WithValue(5.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("signalk").WithType(config.SignalKType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.combined.drive.power").WithValue(11.2),
				),
			),
			false,
		),
		Entry("one path, initialized",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port2.drive.power").WithValue(7.6),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port2.drive.power").WithValue(7.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("signalk").WithType(config.SignalKType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.combined.drive.power").WithValue(13.2),
				),
			),
			false,
		),
		Entry("take most recent relevant time",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port2.drive.power").WithValue(7.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now.Add(time.Second),
				).AddValue(
					message.NewValue().WithPath("propulsion.port3.drive.power").WithValue(8.6),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port2.drive.power").WithValue(7.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now.Add(time.Second),
				).AddValue(
					message.NewValue().WithPath("propulsion.port3.drive.power").WithValue(8.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("signalk").WithType(config.SignalKType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.combined.drive.power").WithValue(13.2),
				),
			),
			false,
		),
		Entry("take most recent time",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port2.drive.power").WithValue(5.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now.Add(time.Second),
				).AddValue(
					message.NewValue().WithPath("propulsion.starboard2.drive.power").WithValue(5.6),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.port2.drive.power").WithValue(5.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now.Add(time.Second),
				).AddValue(
					message.NewValue().WithPath("propulsion.starboard2.drive.power").WithValue(5.6),
				),
			).AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("signalk").WithType(config.SignalKType).WithUuid(uuid.Nil),
				).WithTimestamp(
					now.Add(time.Second),
				).AddValue(
					message.NewValue().WithPath("propulsion.combined.drive.power").WithValue(11.2),
				),
			),
			false,
		),
	)

	It("keeps a path's history bounded to what its own mapping needs, not another mapping's larger retention", func() {
		// Regression test for the old global retentionTime: it was the max
		// RetentionTime across every registered mapping, applied uniformly
		// to every path's history buffer - so one mapping needing a long
		// window (a 60s moving average, say) forced every OTHER path's
		// history to be retained for that same 60s too, even a path with
		// no retention need of its own that arrives far more often (a 2kHz
		// sensor, for instance). test.slowAverage below exists only to
		// register that 60s RetentionTime; its own source path
		// (test.slow) is never fed any data.
		mappings := []*config.ExpressionMappingConfig{
			{
				MappingConfig: config.MappingConfig{
					Path:       "test.historyLength",
					Expression: "len(history['test_fast'])",
				},
				SourcePaths: []string{"test.fast"},
			},
			{
				MappingConfig: config.MappingConfig{
					Path:       "test.slowAverage",
					Expression: "movingAverage(history['test_slow'])",
				},
				SourcePaths:   []string{"test.slow"},
				RetentionTime: 60 * time.Second,
			},
		}
		m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
		Expect(err).ToNot(HaveOccurred())

		var lastLength int
		for i := range 20 {
			// each sample is stamped with the real wall-clock time it's
			// sent at (not a synthetic offset): retention=0 evicts a path's
			// previous entry once real time has moved past its timestamp
			// at all, which a synthetic future-dated timestamp would never
			// satisfy within a fast-running test.
			result, err := m.DoMap(message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType).WithUuid(uuid.Nil),
				).WithTimestamp(time.Now()).AddValue(
					message.NewValue().WithPath("test.fast").WithValue(float64(i)),
				),
			))
			Expect(err).ToNot(HaveOccurred())
			for _, svm := range result.ToSingleValueMapped() {
				if svm.Path == "test.historyLength" {
					lastLength = svm.Value.(int)
				}
			}
		}

		// test.fast's own mapping specifies no retention, so each new
		// sample evicts the previous one immediately: the buffer never
		// grows past the latest entry, regardless of test.slowAverage's
		// unrelated 60s window.
		Expect(lastLength).To(Equal(1))
	})
})
