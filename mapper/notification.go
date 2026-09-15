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

type NotificationMapper struct {
	config               config.MapperConfig
	notificationMappings map[string][]*config.NotificationMappingConfig
	states               map[*config.NotificationMappingConfig]*notificationState
	lastSeen             map[string]time.Time
	env                  ExpressionEnvironment
}

type notificationState struct {
	hasConfirmedState  bool
	confirmedNotifying bool
	pendingNotifying   bool
	pendingSince       time.Time
}

func (s *notificationState) confirm(notifying bool) {
	s.hasConfirmedState = true
	s.confirmedNotifying = notifying
	s.pendingSince = time.Time{}
}

func (s *notificationState) startPending(notifying bool, t time.Time) {
	s.pendingNotifying = notifying
	s.pendingSince = t
}

func (s *notificationState) resetPending() {
	s.pendingSince = time.Time{}
}

func (s *notificationState) reset() (changed bool) {
	changed = s.hasConfirmedState && s.confirmedNotifying
	s.confirm(false)
	return changed
}

func NewNotificationMapper(c config.MapperConfig, nmcs []*config.NotificationMappingConfig) (*NotificationMapper, error) {
	env := NewExpressionEnvironment()

	mappings := make(map[string][]*config.NotificationMappingConfig)
	states := make(map[*config.NotificationMappingConfig]*notificationState, len(nmcs))
	for _, nmc := range nmcs {
		states[nmc] = &notificationState{}
		for _, s := range nmc.SourcePaths {
			mappings[s] = append(mappings[s], nmc)
		}
	}

	return &NotificationMapper{config: c, env: env, notificationMappings: mappings, states: states, lastSeen: make(map[string]time.Time)}, nil
}

// GetTickerInterval returns the interval on which every check should
// additionally be re-evaluated regardless of incoming data, see
// periodicMapper in main.go. Zero disables this.
func (s *NotificationMapper) GetTickerInterval() time.Duration {
	return s.config.Interval
}

// Map subscribes to mapped data and publishes the resulting notifications.
// When config.Interval is set, every check is additionally re-evaluated on
// that interval regardless of incoming data (see refreshMap and
// periodicMapper in main.go), so a check whose source paths have all gone
// completely silent (nothing left to trigger DoMap for it) still gets
// noticed once its Timeout elapses.
func (s *NotificationMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, s, true)
}

// DoMap passes every incoming value through unchanged, in addition to
// publishing any notification a check produces, so the notification mapper
// can sit anywhere in the pipeline without dropping the data it inspects.
// Data for a context other than config.Context is passed through as-is
// without being evaluated by any check, since a check's expression and
// hysteresis state only make sense for the single vessel this mapper is
// configured for.
func (s *NotificationMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	if input.Context != s.config.Context && input.Context != "vessels.self" {
		return input, nil
	}

	result := message.NewMapped().WithContext(s.config.Context).WithOrigin(s.config.Context)

	for _, svm := range input.ToSingleValueMapped() {
		for _, u := range svm.ToMapped().Updates {
			result.AddUpdate(&u)
		}

		nmcs, ok := s.notificationMappings[svm.Path]
		if !ok {
			continue
		}
		path := strings.ReplaceAll(svm.Path, ".", "_")
		s.env[path] = svm
		s.lastSeen[svm.Path] = svm.Timestamp

		for _, nmc := range nmcs {
			if u := s.evaluateCheck(nmc, svm.Timestamp, false); u != nil {
				result.AddUpdate(u)
			}
		}
	}

	return result, nil
}

func (s *NotificationMapper) refreshMap(timeStamp time.Time) *message.Mapped {
	result := message.NewMapped().WithContext(s.config.Context).WithOrigin(s.config.Context)
	for nmc := range s.states {
		if u := s.evaluateCheck(nmc, timeStamp, true); u != nil {
			result.AddUpdate(u)
		}
	}
	return result
}

// evaluateCheck evaluates check as of now, now is either the timestamp of
// the value that triggered the evaluation, or the current time when
// triggered by the periodic sweep. It returns the update to publish, or nil
// when nothing needs to be published.
//
// A confirmed state is otherwise only published once, at the moment it gets
// confirmed: DoMap only calls evaluateCheck when one of the check's source
// paths actually changes, so a check whose confirmed state stays the same
// can go a long time, potentially forever, without evaluateCheck being
// called again for it at all. That single publish is therefore load
// bearing: if it is ever lost downstream (e.g. a subscriber that has not
// finished connecting yet when a freshly started notify confirms a check
// from its very first, fail-safe evaluation, or when a check clears right as
// the process restarts), nothing will tell the rest of the pipeline the
// check's real state, even though it stays correctly confirmed in this
// process's memory the whole time - and unlike an in-memory default, a
// downstream sink that persists the latest value per path (e.g. the
// database writer) keeps serving that stale state forever. republish, set
// by the periodic sweep (see refreshMap and periodicMapper in main.go),
// re-sends the currently confirmed state - notifying or cleared - on every
// sweep regardless of whether it changed, so a lost or stale announcement
// self-heals within one tick interval either way.
func (s *NotificationMapper) evaluateCheck(nmc *config.NotificationMappingConfig, now time.Time, republish bool) *message.Update {
	applies, err := s.applies(nmc)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not evaluate the when expression of a notification, applying the notifcation to be safe",
			zap.String("Path", nmc.Path),
			zap.String("Error", err.Error()),
		)
	}
	u := message.NewUpdate().WithSource(*message.NewSource().WithLabel("notification").WithType(config.SignalKType)).WithTimestamp(now)
	if !applies {
		if changed := s.states[nmc].reset(); !changed && !republish {
			return nil
		}
		u.AddValue(message.NewValue().WithPath(nmc.Path).WithValue(nil))
		return u
	}

	// fail safe: if the expression could not be evaluated, its result is not
	// a bool, or one of the source paths went stale, assume a notification
	// is needed instead of silently skipping it
	rawNotifying := true
	if stalePath, stale := s.staleSourcePath(nmc, now); stale {
		logger.GetLogger().Warn(
			"A source path of a notification has not updated within its timeout, assuming a notification is needed",
			zap.String("Path", nmc.Path),
			zap.String("Stale source path", stalePath),
		)
	} else if value, err := runExpr(s.env, &nmc.MappingConfig); err != nil {
		logger.GetLogger().Warn(
			"Could not evaluate the expression of a notification, assuming a notification is needed",
			zap.String("Path", nmc.Path),
			zap.String("Error", err.Error()),
		)
	} else if v, ok := value.(bool); ok {
		rawNotifying = v
	} else {
		logger.GetLogger().Warn(
			"Could not cast the result of a notification expression to bool, assuming a notification is needed",
			zap.String("Path", nmc.Path),
		)
	}

	notifying, changed := applyHysteresis(nmc, s.states[nmc], rawNotifying, now)
	if !changed && !republish {
		return nil
	}

	if notifying {
		u.AddValue(message.NewValue().WithPath(nmc.Path).WithValue(message.Notification{State: &nmc.State, Method: nmc.Method, Message: &nmc.Message}))
	} else {
		u.AddValue(message.NewValue().WithPath(nmc.Path).WithValue(nil))
	}
	return u
}

func applyHysteresis(nmc *config.NotificationMappingConfig, state *notificationState, observedNotifying bool, timeStamp time.Time) (confirmedNotifying bool, changed bool) {
	if !state.hasConfirmedState {
		state.confirm(observedNotifying)
		return state.confirmedNotifying, true
	}

	if observedNotifying == state.confirmedNotifying {
		state.resetPending()
		return state.confirmedNotifying, false
	}

	if state.pendingSince.IsZero() || state.pendingNotifying != observedNotifying {
		state.startPending(observedNotifying, timeStamp)
	}

	requiredDelay := nmc.ResetDelay
	if observedNotifying {
		requiredDelay = nmc.SetDelay
	}

	if timeStamp.Sub(state.pendingSince) < requiredDelay {
		return state.confirmedNotifying, false
	}

	state.confirm(observedNotifying)
	return state.confirmedNotifying, true
}

// staleSourcePath reports the first of check's SourcePaths that has been
// seen before but not updated for longer than check.Timeout as of now. ok is
// false when Timeout is disabled (zero), or every source path that has been
// seen at least once is still fresh; a source path that has never been seen
// is not considered stale, that case is left to the expression to fail on.
func (s *NotificationMapper) staleSourcePath(check *config.NotificationMappingConfig, now time.Time) (path string, ok bool) {
	if check.Timeout <= 0 {
		return "", false
	}
	for _, p := range check.SourcePaths {
		if last, seen := s.lastSeen[p]; seen && now.Sub(last) >= check.Timeout {
			return p, true
		}
	}
	return "", false
}

func (s *NotificationMapper) applies(nmc *config.NotificationMappingConfig) (bool, error) {
	if nmc.When == "" {
		return true, nil
	}

	if nmc.CompiledWhen == nil {
		var err error
		if nmc.CompiledWhen, err = expr.Compile(nmc.When); err != nil {
			return true, err
		}
	}

	output, err := virtualMachine.Run(nmc.CompiledWhen, s.env)
	if err != nil {
		return true, err
	}

	applies, ok := output.(bool)
	if !ok {
		return true, fmt.Errorf("could not cast result of the when expression to bool")
	}

	return applies, nil
}
