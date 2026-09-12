package mapper_test

import (
	"time"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DoMap alarm", func() {
	mapper, _ := NewAlarmMapper(
		config.NewAlarmMappingConfig("alarm_test.yaml"),
	)
	now := time.Now()
	insane := true
	sane := false
	powerCheckExpr := "propulsion_mainEngine_drive_power.Value < propulsion_mainEngine_fuel_rate.Value / 0.25 or propulsion_mainEngine_drive_power.Value > propulsion_mainEngine_fuel_rate.Value / 0.2"
	supplyReturnCheckExpr := "propulsion_mainEngine_fuel_rate_supply.Value < propulsion_mainEngine_fuel_rate_return.Value"
	thresholdCheckExpr := "test_threshold.Value > 100"
	whenMissingCheckExpr := "test_whenMissingValue.Value > 100"
	base := now.Add(1000 * time.Second)

	DescribeTable("Messages",
		func(m *AlarmMapper, input *message.Mapped, expected *message.Mapped, expectError bool) {
			result, err := m.DoMap(input)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expected))
			}
		},
		Entry("fuel rate arrives, power is not known yet, check is skipped, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate").WithValue(20000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("power matches the fuel rate, first observation is reported immediately as sane",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue(90000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(now).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(
						message.Notification{State: &sane, Message: strPtr("check passed for notifications.propulsion.mainEngine.drive.power: " + powerCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("power drops too low, hysteresis has not passed yet, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue(70000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("power is still too low 30s later, hysteresis still has not passed, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(30*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue(70000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("power has been too low for over a minute, hysteresis passed, now reported as insane",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue(70000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(
						message.Notification{State: &insane, Message: strPtr("check failed for notifications.propulsion.mainEngine.drive.power: " + powerCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("power is too high while already confirmed insane, insane notifications keep repeating",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue(110000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(
						message.Notification{State: &insane, Message: strPtr("check failed for notifications.propulsion.mainEngine.drive.power: " + powerCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("power recovers, reported as sane immediately without waiting for hysteresis",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue(90000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(
						message.Notification{State: &sane, Message: strPtr("check passed for notifications.propulsion.mainEngine.drive.power: " + powerCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("no fuel is being consumed, the check does not apply, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate").WithValue(0.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("supply fuel rate arrives, return fuel rate is not known yet, check is skipped, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate.supply").WithValue(20.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("return fuel rate is lower than the supply fuel rate, first observation is reported immediately as sane",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate.return").WithValue(5.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(
						message.Notification{State: &sane, Message: strPtr("check passed for notifications.propulsion.mainEngine.fuel.supplyReturn: " + supplyReturnCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("supply fuel rate drops below the return fuel rate, hysteresis has not passed yet, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate.supply").WithValue(2.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("supply fuel rate has stayed below the return fuel rate for over a minute, hysteresis passed, now reported as insane",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate.supply").WithValue(2.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(
						message.Notification{State: &insane, Message: strPtr("check failed for notifications.propulsion.mainEngine.fuel.supplyReturn: " + supplyReturnCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("supply fuel rate is still below the return fuel rate, insane notifications keep repeating",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate.supply").WithValue(2.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(
						message.Notification{State: &insane, Message: strPtr("check failed for notifications.propulsion.mainEngine.fuel.supplyReturn: " + supplyReturnCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("return fuel rate drops, supply is higher again, reported as sane immediately without waiting for hysteresis",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate.return").WithValue(1.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(
						message.Notification{State: &sane, Message: strPtr("check passed for notifications.propulsion.mainEngine.fuel.supplyReturn: " + supplyReturnCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("threshold check, first observation is reported immediately as sane",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(50.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("notifications.test.threshold").WithValue(
						message.Notification{State: &sane, Message: strPtr("check passed for notifications.test.threshold: " + thresholdCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("threshold goes over, setHysteresis has not passed yet, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(150.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("threshold is still over 29s later, setHysteresis still has not passed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base.Add(29*time.Second)).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(150.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("threshold has been over for 31s, setHysteresis passed, now reported as insane",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base.Add(31*time.Second)).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(150.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(base.Add(31*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.test.threshold").WithValue(
						message.Notification{State: &insane, Message: strPtr("check failed for notifications.test.threshold: " + thresholdCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("threshold recovers, resetHysteresis has not passed yet, insane notification keeps repeating",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base.Add(32*time.Second)).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(50.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(base.Add(32*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.test.threshold").WithValue(
						message.Notification{State: &insane, Message: strPtr("check failed for notifications.test.threshold: " + thresholdCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("threshold is still sane 19s later, resetHysteresis still has not passed, insane notification keeps repeating",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base.Add(51*time.Second)).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(50.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(base.Add(51*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.test.threshold").WithValue(
						message.Notification{State: &insane, Message: strPtr("check failed for notifications.test.threshold: " + thresholdCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("threshold has stayed sane for 21s, resetHysteresis passed, now reported as sane",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base.Add(53*time.Second)).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(50.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(base.Add(53*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.test.threshold").WithValue(
						message.Notification{State: &sane, Message: strPtr("check passed for notifications.test.threshold: " + thresholdCheckExpr)},
					),
				),
			),
			false,
		),
		Entry("when expression cannot be evaluated, the check fails safe and is applied anyway",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.whenMissingValue").WithValue(150.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("alarm").WithType(config.SignalKType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("notifications.test.whenMissing").WithValue(
						message.Notification{State: &insane, Message: strPtr("check failed for notifications.test.whenMissing: " + whenMissingCheckExpr)},
					),
				),
			),
			false,
		),
	)
})

func strPtr(s string) *string {
	return &s
}
