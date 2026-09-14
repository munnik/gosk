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
				passThroughUpdate("propulsion.mainEngine.fuel.rate", 20000.0, now),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("propulsion.mainEngine.fuel.rate", 20000.0, now)).
				AddUpdate(
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
				passThroughUpdate("propulsion.mainEngine.drive.power", 90000.0, now),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("propulsion.mainEngine.drive.power", 90000.0, now)).
				AddUpdate(
					message.NewUpdate().WithSource(
						*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
					).WithTimestamp(now).AddValue(
						message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(nil),
					),
				),
			false,
		),
		Entry("power drops too low, hysteresis has not passed yet, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.drive.power", 70000.0, now),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.drive.power", 70000.0, now),
			),
			false,
		),
		Entry("power is still too low 30s later, hysteresis still has not passed, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.drive.power", 70000.0, now.Add(30*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.drive.power", 70000.0, now.Add(30*time.Second)),
			),
			false,
		),
		Entry("power has been too low for over a minute, hysteresis passed, the notification is now raised",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.drive.power", 70000.0, now.Add(61*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("propulsion.mainEngine.drive.power", 70000.0, now.Add(61*time.Second))).
				AddUpdate(
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
		Entry("power is too high while the notification is already confirmed raised, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.drive.power", 110000.0, now.Add(61*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.drive.power", 110000.0, now.Add(61*time.Second)),
			),
			false,
		),
		Entry("power recovers, the notification is cleared immediately without waiting for hysteresis",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.drive.power", 90000.0, now.Add(61*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("propulsion.mainEngine.drive.power", 90000.0, now.Add(61*time.Second))).
				AddUpdate(
					message.NewUpdate().WithSource(
						*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
					).WithTimestamp(now.Add(61*time.Second)).AddValue(
						message.NewValue().WithPath("notifications.propulsion.mainEngine.drive.power").WithValue(nil),
					),
				),
			false,
		),
		Entry("no fuel is being consumed, the check does not apply, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate", 0.0, now.Add(61*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate", 0.0, now.Add(61*time.Second)),
			),
			false,
		),
		Entry("supply fuel rate arrives, return fuel rate is not known yet, the expression cannot be evaluated, the check fails safe and raises the notification",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate.supply", 20.0, now.Add(61*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("propulsion.mainEngine.fuel.rate.supply", 20.0, now.Add(61*time.Second))).
				AddUpdate(
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
				passThroughUpdate("propulsion.mainEngine.fuel.rate.return", 5.0, now.Add(61*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("propulsion.mainEngine.fuel.rate.return", 5.0, now.Add(61*time.Second))).
				AddUpdate(
					message.NewUpdate().WithSource(
						*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
					).WithTimestamp(now.Add(61*time.Second)).AddValue(
						message.NewValue().WithPath("notifications.propulsion.mainEngine.fuel.supplyReturn").WithValue(nil),
					),
				),
			false,
		),
		Entry("supply fuel rate drops below the return fuel rate, hysteresis has not passed yet, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate.supply", 2.0, now.Add(61*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate.supply", 2.0, now.Add(61*time.Second)),
			),
			false,
		),
		Entry("supply fuel rate has stayed below the return fuel rate for over a minute, hysteresis passed, the notification is now raised",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate.supply", 2.0, now.Add(122*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("propulsion.mainEngine.fuel.rate.supply", 2.0, now.Add(122*time.Second))).
				AddUpdate(
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
		Entry("supply fuel rate is still below the return fuel rate, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate.supply", 2.0, now.Add(122*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate.supply", 2.0, now.Add(122*time.Second)),
			),
			false,
		),
		Entry("return fuel rate drops, supply is higher again, the notification is cleared immediately without waiting for hysteresis",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("propulsion.mainEngine.fuel.rate.return", 1.0, now.Add(122*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("propulsion.mainEngine.fuel.rate.return", 1.0, now.Add(122*time.Second))).
				AddUpdate(
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
				passThroughUpdate("test.threshold", 50.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.threshold", 50.0, base)).
				AddUpdate(
					message.NewUpdate().WithSource(
						*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
					).WithTimestamp(base).AddValue(
						message.NewValue().WithPath("notifications.test.threshold").WithValue(nil),
					),
				),
			false,
		),
		Entry("threshold goes over, setHysteresis has not passed yet, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 150.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 150.0, base),
			),
			false,
		),
		Entry("threshold is still over 29s later, setHysteresis still has not passed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 150.0, base.Add(29*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 150.0, base.Add(29*time.Second)),
			),
			false,
		),
		Entry("threshold has been over for 31s, setHysteresis passed, the notification is now raised",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 150.0, base.Add(31*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.threshold", 150.0, base.Add(31*time.Second))).
				AddUpdate(
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
		Entry("threshold recovers, resetHysteresis has not passed yet, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 50.0, base.Add(32*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 50.0, base.Add(32*time.Second)),
			),
			false,
		),
		Entry("threshold is still cleared 19s later, resetHysteresis still has not passed, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 50.0, base.Add(51*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 50.0, base.Add(51*time.Second)),
			),
			false,
		),
		Entry("threshold has stayed cleared for 21s, resetHysteresis passed, the notification is confirmed cleared",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.threshold", 50.0, base.Add(53*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.threshold", 50.0, base.Add(53*time.Second))).
				AddUpdate(
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
				passThroughUpdate("test.stateAndMethod", 150.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.stateAndMethod", 150.0, base)).
				AddUpdate(
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
				passThroughUpdate("test.whenMissingValue", 150.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.whenMissingValue", 150.0, base)).
				AddUpdate(
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
				passThroughUpdate("test.whenResetGate", 1.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.whenResetGate", 1.0, base)).
				AddUpdate(
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
		Entry("value exceeds the threshold while the gate is open, the notification was already raised by the earlier fail-safe, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.whenReset", 150.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.whenReset", 150.0, base),
			),
			false,
		),
		Entry("gate closes while the notification is confirmed raised, the check is reset and the notification is cleared",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.whenResetGate", 0.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.whenResetGate", 0.0, base)).
				AddUpdate(
					message.NewUpdate().WithSource(
						*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
					).WithTimestamp(base).AddValue(
						message.NewValue().WithPath("notifications.test.whenReset").WithValue(nil),
					),
				),
			false,
		),
		Entry("gate stays closed, already reset, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.whenResetGate", 0.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.whenResetGate", 0.0, base),
			),
			false,
		),
		Entry("the expression does not evaluate to a bool, the check fails safe and raises the notification",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.castFails", 42.0, base),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.castFails", 42.0, base)).
				AddUpdate(
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
				passThroughUpdate("test.timeoutPrimary", 50.0, timeoutBase),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.timeoutPrimary", 50.0, timeoutBase)).
				AddUpdate(
					message.NewUpdate().WithSource(
						*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
					).WithTimestamp(timeoutBase).AddValue(
						message.NewValue().WithPath("notifications.test.timeout").WithValue(nil),
					),
				),
			false,
		),
		Entry("timeout: companion arrives too, still fresh, state has not changed, only the value is passed through",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.timeoutSecondary", 1.0, timeoutBase),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.timeoutSecondary", 1.0, timeoutBase),
			),
			false,
		),
		Entry("timeout: companion has not updated within its timeout, the check fails safe and raises the notification",
			mapper,
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").AddUpdate(
				passThroughUpdate("test.timeoutPrimary", 50.0, timeoutBase.Add(40*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.timeoutPrimary", 50.0, timeoutBase.Add(40*time.Second))).
				AddUpdate(
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
				passThroughUpdate("test.timeoutSecondary", 1.0, timeoutBase.Add(40*time.Second)),
			),
			message.NewMapped().WithContext("testingContext").WithOrigin("testingContext").
				AddUpdate(passThroughUpdate("test.timeoutSecondary", 1.0, timeoutBase.Add(40*time.Second))).
				AddUpdate(
					message.NewUpdate().WithSource(
						*message.NewSource().WithLabel("notification").WithType(config.SignalKType),
					).WithTimestamp(timeoutBase.Add(40*time.Second)).AddValue(
						message.NewValue().WithPath("notifications.test.timeout").WithValue(nil),
					),
				),
			false,
		),
		Entry("data for another context is passed through as-is, no check is evaluated for it",
			mapper,
			message.NewMapped().WithContext("someOtherContext").WithOrigin("someOtherContext").AddUpdate(
				passThroughUpdate("test.threshold", 300.0, base),
			),
			message.NewMapped().WithContext("someOtherContext").WithOrigin("someOtherContext").AddUpdate(
				passThroughUpdate("test.threshold", 300.0, base),
			),
			false,
		),
	)
})

// passThroughUpdate builds the update NotificationMapper's DoMap emits to
// carry an incoming value through unchanged, so a test's expected result can
// be composed from it plus whatever notification the check also produced.
func passThroughUpdate(path string, value interface{}, at time.Time) *message.Update {
	return message.NewUpdate().WithSource(
		*message.NewSource().WithLabel("testingConnector").WithType(config.JSONType),
	).WithTimestamp(at).AddValue(
		message.NewValue().WithPath(path).WithValue(value),
	)
}

func strPtr(s string) *string {
	return &s
}
