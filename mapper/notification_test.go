package mapper_test

import (
	"time"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DoMap notification", func() {
	mapper, _ := NewNotificationMapper(
		config.MapperConfig{Context: "testingContext"},
		config.NewNotificationMappingConfig("notification_test.yaml"),
	)
	now := time.Now()
	state := "alarm"
	defaultMethod := []string{"sound", "visual"}
	powerMessage := "drive power does not match the expected fuel rate ratio"
	supplyReturnMessage := "fuel supply rate is lower than the return rate"
	thresholdMessage := "test threshold exceeded"
	whenMissingMessage := "test whenMissing value exceeded"
	whenResetMessage := "test whenReset value exceeded"
	castFailsMessage := "test castFails expression did not evaluate to bool"
	timeoutMessage := "test timeout companion source path went stale"
	base := now.Add(1000 * time.Second)
	timeoutBase := base.Add(2000 * time.Second)

	DescribeTable("Messages",
		func(m *NotificationMapper, input *message.Mapped, expected *message.Mapped, expectError bool) {
			result, err := m.DoMap(input)
			if expectError {
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			} else {
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(Equal(expected))
			}
		},
		Entry("fuel rate arrives, power is not known yet, the expression cannot be evaluated, the check fails safe and raises the notification",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate").WithValue(20000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(now).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &powerMessage},
					),
				),
			),
			false,
		),
		Entry("power matches the fuel rate, the notification is cleared immediately",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(now).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(nil),
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
		Entry("power has been too low for over a minute, hysteresis passed, the notification is now raised",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &powerMessage},
					),
				),
			),
			false,
		),
		Entry("power is too high while the notification is already confirmed raised, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.drive.power").WithValue(110000.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("power recovers, the notification is cleared immediately without waiting for hysteresis",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(nil),
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
		Entry("supply fuel rate arrives, return fuel rate is not known yet, the expression cannot be evaluated, the check fails safe and raises the notification",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate.supply").WithValue(20.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &supplyReturnMessage},
					),
				),
			),
			false,
		),
		Entry("return fuel rate is lower than the supply fuel rate, the notification is cleared immediately",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(now.Add(61*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(nil),
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
		Entry("supply fuel rate has stayed below the return fuel rate for over a minute, hysteresis passed, the notification is now raised",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &supplyReturnMessage},
					),
				),
			),
			false,
		),
		Entry("supply fuel rate is still below the return fuel rate, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("propulsion.mainEngine.fuel.rate.supply").WithValue(2.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("return fuel rate drops, supply is higher again, the notification is cleared immediately without waiting for hysteresis",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(now.Add(122*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(nil),
				),
			),
			false,
		),
		Entry("threshold check, first observation is confirmed cleared immediately",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("notifications.test.threshold").WithValue(nil),
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
		Entry("threshold has been over for 31s, setHysteresis passed, the notification is now raised",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(base.Add(31*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.test.threshold").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &thresholdMessage},
					),
				),
			),
			false,
		),
		Entry("threshold recovers, resetHysteresis has not passed yet, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base.Add(32*time.Second)).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(50.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("threshold is still cleared 19s later, resetHysteresis still has not passed, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base.Add(51*time.Second)).AddValue(
					message.NewValue().WithPath("test.threshold").WithValue(50.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("threshold has stayed cleared for 21s, resetHysteresis passed, the notification is confirmed cleared",
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(base.Add(53*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.test.threshold").WithValue(nil),
				),
			),
			false,
		),
		Entry("configured state and method are used instead of the defaults",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.stateAndMethod").WithValue(150.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("notifications.test.stateAndMethod").WithValue(
						message.Notification{
							State:   strPtr("emergency"),
							Method:  []string{"sound", "visual"},
							Message: strPtr("test stateAndMethod value exceeded"),
						},
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
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("notifications.test.whenMissing").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &whenMissingMessage},
					),
				),
			),
			false,
		),
		Entry("when gate opens, value is not known yet, the expression cannot be evaluated, the check fails safe and raises the notification",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.whenResetGate").WithValue(1.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("notifications.test.whenReset").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &whenResetMessage},
					),
				),
			),
			false,
		),
		Entry("value exceeds the threshold while the gate is open, the notification was already raised by the earlier fail-safe, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.whenReset").WithValue(150.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("gate closes while the notification is confirmed raised, the check is reset and the notification is cleared",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.whenResetGate").WithValue(0.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("notifications.test.whenReset").WithValue(nil),
				),
			),
			false,
		),
		Entry("gate stays closed, already reset, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.whenResetGate").WithValue(0.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("the expression does not evaluate to a bool, the check fails safe and raises the notification",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("test.castFails").WithValue(42.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(base).AddValue(
					message.NewValue().WithPath("notifications.test.castFails").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &castFailsMessage},
					),
				),
			),
			false,
		),
		Entry("timeout: primary arrives, companion has never been seen, first observation is confirmed cleared immediately",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(timeoutBase).AddValue(
					message.NewValue().WithPath("test.timeoutPrimary").WithValue(50.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(timeoutBase).AddValue(
					message.NewValue().WithPath("notifications.test.timeout").WithValue(nil),
				),
			),
			false,
		),
		Entry("timeout: companion arrives too, still fresh, state has not changed, nothing is output",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(timeoutBase).AddValue(
					message.NewValue().WithPath("test.timeoutSecondary").WithValue(1.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext"),
			false,
		),
		Entry("timeout: companion has not updated within its timeout, the check fails safe and raises the notification",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(timeoutBase.Add(40*time.Second)).AddValue(
					message.NewValue().WithPath("test.timeoutPrimary").WithValue(50.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(timeoutBase.Add(40*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.test.timeout").WithValue(
						message.Notification{State: &state, Method: defaultMethod, Message: &timeoutMessage},
					),
				),
			),
			false,
		),
		Entry("timeout: companion updates again, the notification is cleared",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
				).WithTimestamp(timeoutBase.Add(40*time.Second)).AddValue(
					message.NewValue().WithPath("test.timeoutSecondary").WithValue(1.0),
				),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				message.NewUpdate().WithSource(
					*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
				).WithTimestamp(timeoutBase.Add(40*time.Second)).AddValue(
					message.NewValue().WithPath("notifications.test.timeout").WithValue(nil),
				),
			),
			false,
		),
	)
})

func strPtr(s string) *string {
	return &s
}
