package mapper

import (
	"fmt"
	"strconv"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type BinaryMapper struct {
	config             config.MapperConfig
	protocol           string
	mappingsConfig     []config.MappingConfig
	notifications      notificationEvaluator
	singleTorqueSensor bool
	peakToPeak         *peakToPeakRpm
	env                ExpressionEnvironment
}

func NewBinaryMapper(
	c config.MapperConfig,
	mc []config.MappingConfig,
	nc []*config.NotificationMappingConfig,
) (*BinaryMapper, error) {
	for i := range mc {
		precompileMapping(&mc[i])
	}
	singleTorqueSensor := false
	if _, ok := c.ProtocolOptions[config.ProtocolOptionBinarySingleTorqueSensor]; ok {
		singleTorqueSensor, _ = strconv.ParseBool(c.ProtocolOptions[config.ProtocolOptionBinarySingleTorqueSensor])
	}
	return &BinaryMapper{
		config:             c,
		protocol:           config.BinaryType,
		mappingsConfig:     mc,
		notifications:      newNotificationEvaluator(nc),
		singleTorqueSensor: singleTorqueSensor,
		peakToPeak:         &peakToPeakRpm{},
		env:                NewExpressionEnvironment(),
	}, nil
}

// MapConnectorStatus implements ConnectorStatusMapper - see its doc
// comment and process in main.go.
func (m *BinaryMapper) MapConnectorStatus(connector string, connected bool) *message.Mapped {
	return NewConnectorStatusUpdate(m.config.Context, connector, connected)
}

// GetTickerInterval returns the interval on which the mapper should
// additionally be re-evaluated regardless of incoming data, see
// periodicMapper in main.go. Zero disables this.
func (m *BinaryMapper) GetTickerInterval() time.Duration {
	return m.config.Interval
}

func (m *BinaryMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

// mannerSensorDiffExpression, mannerCombinedTorqueExpression, and
// mannerRpmEstimateEnvVar are hardcoded, not configurable: every
// BinaryMapper decodes a Manner shaft power meter frame (it's the only user
// of the "binary" protocol fleet-wide). Computing the estimate is
// therefore unconditional, not something a check opts into: nix never
// needs to mention EnvVar, it costs nothing on a frame nothing reads it
// from, and any notification that wants it can just reference it in its
// own Expression.
//
// Which of the two input signals feeds peakToPeakRpm depends on
// singleTorqueSensor (config.ProtocolOptionBinarySingleTorqueSensor, since
// that's a fact about the physical installation nix already knows, not
// something gosk can detect from the byte stream):
//
//   - Two sensors (the default): bytes 2-3 and 4-5 are the shaft's two
//     torque sensors, mounted 180 degrees apart. Pure torque is the same
//     reading at both (shear strain from torque is uniform around the
//     shaft's circumference), so their difference cancels it out entirely
//     and leaves only the once-per-revolution bending signal - clean,
//     unaffected by torque changing with engine load (see
//     notificationsForMannerTcpSensorHealth's doc comment in ../nix).
//   - One sensor: bytes 10-11 are the frame's single combined, calibrated
//     torque reading (the same bytes mappingsForMannerTcp publishes as
//     drive.torque). It still carries the same once-per-revolution bending
//     ripple - any real shaft has some bending - just superimposed on the
//     torque signal instead of isolated from it, so peak/trough timing is
//     noisier: a torque change happening within a couple of revolutions
//     (e.g. the throttle opening) can distort or mask a turning point in a
//     way the two-sensor difference is immune to by construction.
const (
	mannerSensorDiffExpression     = "float(toUInt(value[2], value[3])) - float(toUInt(value[4], value[5]))"
	mannerCombinedTorqueExpression = "float(toUInt(value[10], value[11]))"
	mannerRpmEstimateEnvVar        = "estimatedRevolutions"
)

var (
	mannerSensorDiffMappingConfig     = &config.MappingConfig{Expression: mannerSensorDiffExpression}
	mannerCombinedTorqueMappingConfig = &config.MappingConfig{Expression: mannerCombinedTorqueExpression}
)

// DoMap maps r's raw bytes according to mappingsConfig, same as always,
// publishes a peak-to-peak speed estimate (see peakToPeakRpm) into env
// under mannerRpmEstimateEnvVar, and evaluates notifications against that
// same env (env's "value" plus mannerRpmEstimateEnvVar), so a check can
// compare readings - or the estimate - this protocol derives from the
// frame without any of them ever having to be published as their own
// SignalK value first.
func (m *BinaryMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)
	m.env["value"] = r.Value
	// By index rather than by value: &mc would point at the loop's copy,
	// so runExpr's compile cache would be written to something discarded
	// at the end of the iteration. This used to be worked around by
	// writing the copy back with m.mappingsConfig[i] = mc, which only
	// took effect for a mapping that had already produced a value once.
	// NewBinaryMapper now compiles them all up front, so there is nothing
	// left to write back.
	for i := range m.mappingsConfig {
		mc := &m.mappingsConfig[i]
		output, err := runExpr(m.env, mc)
		if err == nil {
			u.AddValue(message.NewValue().WithPath(mc.Path).WithValue(output))
		}
	}
	if len(u.Values) > 0 {
		result.AddUpdate(u)
	}

	peakToPeakSource := mannerSensorDiffMappingConfig
	if m.singleTorqueSensor {
		peakToPeakSource = mannerCombinedTorqueMappingConfig
	}
	evaluatePeakToPeak(m.env, peakToPeakSource, m.peakToPeak, mannerRpmEstimateEnvVar, r.Timestamp)
	m.notifications.evaluate(m.env, r.Timestamp, result)

	if len(result.Updates) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result, nil
}
