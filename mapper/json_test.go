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

var _ = Describe("DoMap json", func() {
	mapper, _ := NewJSONMapper(
		config.MapperConfig{Context: "testingContext"},
		config.NewJSONMappingConfig("json_test.yaml"),
	)
	now := time.Now()

	DescribeTable("Messages",
		func(m *JSONMapper, input *message.Raw, expected *message.Mapped, expectError bool) {
			result, err := m.DoMap(input)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expected))
			}
		},
		Entry("With empty value",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.JSONType).WithValue([]byte{})
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			nil,
			true,
		),
		Entry("With torque json",
			mapper,
			func() *message.Raw {
				m := message.NewRaw().WithCollector("testingCollector").WithType(config.JSONType).WithValue([]byte(`{"hn":"DERR1","seq":"19218188","trq":"82.29","spd":"980","pwr":"8409.6","tmp":"26.9","sps":"1000","ax":"16","ay":"8","az":"262","dt":"20180404143809"}`))
				m.Uuid = uuid.Nil
				m.Timestamp = now
				return m
			}(),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingCollector").WithType(config.JSONType),
				).WithTimestamp(
					now,
				).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithUuid(uuid.Nil).WithValue("8409.6"),
				),
			),
			false,
		),
	)
})
