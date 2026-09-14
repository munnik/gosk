package mapper

import (
	"fmt"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

// AlarmMapper subscribes to mapped data and checks it for sane values.
// A check is a boolean expression over one or more paths, e.g. comparing a
// ratio or a difference between two paths against expected bounds. It
// evaluates to true when the value is insane and false when it is sane.
// Checks can also be limited to when a condition applies, e.g. only check
// the fuel rate when the engine is actually running. Becoming insane is only
// notified once the insane value has been observed continuously for the
// check's configured SetDelay, and recovering back to sane is only notified
// once the sane value has been observed continuously for the check's
// configured ResetDelay, this avoids flapping notifications for values close
// to the boundary. A notification is only published when the confirmed state
// changes, plus once for the very first observation (the state is not known
// yet, e.g. right after the mapper starts) - it is never repeated while the
// confirmed state stays the same. The result of a check is published as a
// notification, the original data is not passed through. If no check
// produced a notification nothing is output.
type AlarmMapper struct {
	checkMappings map[string][]*config.AlarmMappingConfig
	env           ExpressionEnvironment
}

func NewAlarmMapper(scc []*config.AlarmMappingConfig) (*AlarmMapper, error) {
	env := NewExpressionEnvironment()

	mappings := make(map[string][]*config.AlarmMappingConfig)
	for _, c := range scc {
		for _, s := range c.SourcePaths {
			mappings[s] = append(mappings[s], c)
		}
	}

	return &AlarmMapper{env: env, checkMappings: mappings}, nil
}

func (s *AlarmMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, s, true)
}

func (s *AlarmMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(input.Context).WithOrigin(input.Origin)

	for _, svm := range input.ToSingleValueMapped() {
		checks, ok := s.checkMappings[svm.Path]
		if !ok {
			continue
		}
		path := strings.ReplaceAll(svm.Path, ".", "_")
		s.env[path] = svm

		for _, check := range checks {
			applies, err := s.applies(check)
			if err != nil {
				// fail safe: if the when expression could not be evaluated, the
				// check is applied anyway instead of silently skipping the alarm
				logger.GetLogger().Warn(
					"Could not evaluate the when expression of an alarm check, applying the check to be safe",
					zap.String("Path", check.Path),
					zap.String("Error", err.Error()),
				)
			}
			if !applies {
				continue
			}

			value, err := runExpr(s.env, &check.MappingConfig)
			if err != nil {
				logger.GetLogger().Debug(
					"Could not evaluate the expression of an alarm check, skipping the check",
					zap.String("Path", check.Path),
					zap.String("Error", err.Error()),
				)
				continue
			}
			rawInsane, ok := value.(bool)
			if !ok {
				logger.GetLogger().Warn(
					"Could not cast the result of an alarm check expression to bool",
					zap.String("Path", check.Path),
				)
				continue
			}

			insane, changed := applyHysteresis(check, rawInsane, svm.Timestamp)
			if !changed {
				// confirmed state has not changed, do not repeat the notification
				continue
			}

			m := check.Message
			if m == "" {
				if insane {
					m = fmt.Sprintf("check failed for %s: %s", check.Path, check.Expression)
				} else {
					m = fmt.Sprintf("check passed for %s: %s", check.Path, check.Expression)
				}
			}
			u := message.NewUpdate().WithSource(*message.NewSource().WithLabel("alarm").WithType(config.SignalKType)).WithTimestamp(svm.Timestamp)
			u.AddValue(message.NewValue().WithPath(check.Path).WithValue(message.Notification{State: &insane, Message: &m}))
			result.AddUpdate(u)
		}
	}

	return result, nil
}

// applyHysteresis debounces state changes: rawInsane must be observed
// continuously for check.SetDelay before becoming insane is confirmed, and a
// sane rawInsane must be observed continuously for check.ResetDelay before
// recovering back to sane is confirmed, this avoids flapping notifications
// when the value is close to the boundary. It returns the confirmed state
// and whether that state just changed (this is also true for the very first
// observation, since the state was not known before that). Callers should
// notify only when changed is true, and otherwise skip notifying so a
// confirmed state is not repeated on every update.
func applyHysteresis(check *config.AlarmMappingConfig, rawInsane bool, at time.Time) (insane bool, changed bool) {
	if !check.Notified {
		check.Notified = true
		check.ConfirmedInsane = rawInsane
		check.PendingSince = time.Time{}
		return check.ConfirmedInsane, true
	}

	if rawInsane == check.ConfirmedInsane {
		// matches the confirmed state, no state change is pending
		check.PendingSince = time.Time{}
		return check.ConfirmedInsane, false
	}

	if check.PendingSince.IsZero() || check.PendingInsane != rawInsane {
		// raw value just diverged from the confirmed state, or flipped again
		// while a transition was already pending, (re)start the timer
		check.PendingInsane = rawInsane
		check.PendingSince = at
	}

	delay := check.ResetDelay
	if rawInsane {
		delay = check.SetDelay
	}

	if at.Sub(check.PendingSince) >= delay {
		check.ConfirmedInsane = rawInsane
		check.PendingSince = time.Time{}
		return rawInsane, true
	}

	return check.ConfirmedInsane, false
}

// applies evaluates the When expression of a check, an empty When always applies.
// If the expression cannot be compiled or run, or does not evaluate to a bool,
// it fails safe: it returns true (the check applies) together with the error,
// so a broken when expression does not silently suppress the alarm.
func (s *AlarmMapper) applies(check *config.AlarmMappingConfig) (bool, error) {
	if check.When == "" {
		return true, nil
	}

	if check.CompiledWhen == nil {
		var err error
		if check.CompiledWhen, err = expr.Compile(check.When); err != nil {
			return true, err
		}
	}

	output, err := virtualMachine.Run(check.CompiledWhen, s.env)
	if err != nil {
		return true, err
	}

	applies, ok := output.(bool)
	if !ok {
		return true, fmt.Errorf("could not cast result of the when expression to bool")
	}

	return applies, nil
}
