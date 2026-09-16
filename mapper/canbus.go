package mapper

import (
	"io"
	"os"
	"slices"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"

	"go.einride.tech/can"
	"go.einride.tech/can/pkg/dbc"
	"go.einride.tech/can/pkg/descriptor"
)

type CanBusMapper struct {
	config         config.CanBusMapperConfig
	protocol       string
	dbc            DBC
	canbusMappings map[string]map[string]config.CanBusMappingConfig
	notifications  notificationEvaluator
	// peakToPeaks is keyed by signal origin (e.g. "Raw_Data_0"), one per
	// engine: a dual-engine vessel (two Manner units on one CAN bus) gets
	// two independent estimates rather than one contaminated by both.
	peakToPeaks map[string]*peakToPeakRpm
	// env persists across frames, unlike "value" below, which is only ever
	// the single signal currently being run through canbusMappings - so a
	// notification can reference a signal decoded from an earlier,
	// different CAN message (e.g. this engine's last known RPM, which
	// arrives on a different message than torque).
	env ExpressionEnvironment
}

type signal struct {
	origin string
	name   string
	value  float64
	valid  bool
}

type DBC map[uint32]*dbc.MessageDef

func NewCanBusMapper(
	c config.CanBusMapperConfig,
	cmc []config.CanBusMappingConfig,
	nc []*config.NotificationMappingConfig,
) (*CanBusMapper, error) {
	// parse DBC file and store mappings
	dbc := readDBC(c.DbcFile, c.IsJ1939)
	// Before the loop below copies them into the map - a struct stored
	// in a map is not addressable, so this cannot be done afterwards.
	for i := range cmc {
		precompileMapping(&cmc[i].MappingConfig)
	}

	mappings := make(map[string]map[string]config.CanBusMappingConfig)
	for _, m := range cmc {
		_, present := mappings[m.Origin]
		if !present {
			mappings[m.Origin] = make(map[string]config.CanBusMappingConfig)
		}
		mappings[m.Origin][m.Name] = m
	}
	return &CanBusMapper{
		config:         c,
		protocol:       config.CanBusType,
		dbc:            dbc,
		canbusMappings: mappings,
		notifications:  newNotificationEvaluator(nc),
		peakToPeaks:    make(map[string]*peakToPeakRpm),
		env:            NewExpressionEnvironment(),
	}, nil
}

// MapConnectorStatus implements ConnectorStatusMapper - see its doc
// comment and process in main.go.
func (m *CanBusMapper) MapConnectorStatus(connector string, connected bool) *message.Mapped {
	return NewConnectorStatusUpdate(m.config.Context, connector, connected)
}

// GetTickerInterval returns the interval on which the mapper should
// additionally be re-evaluated regardless of incoming data, see
// periodicMapper in main.go. Zero disables this.
func (m *CanBusMapper) GetTickerInterval() time.Duration {
	return m.config.Interval
}

func (m *CanBusMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

// mannerTorqueSumSignalName is hardcoded, not configurable: it's the name
// dbc.nix's generated DBC always gives the Manner shaft power meter's
// combined torque signal, on whichever origin carries it (one per engine -
// "Raw_Data_0", "Raw_Data_1", ...). That combined signal completes one
// sinusoidal cycle per shaft revolution the same way it does for the TCP
// variant (see notificationsForMannerTcpSensorHealth's doc comment in
// ../nix), and unlike the two individual sensor signals it's always
// present regardless of whether one or two torque sensors are physically
// installed (see notificationsForMannerCanbusSensorHealth in ../nix), so
// it's what feeds peakToPeakRpm here.
const mannerTorqueSumSignalName = "Torque_Sum_Raw_Data"

// mannerRpmEstimateEnvVarSuffix: a peak-to-peak estimate is published into
// env under mannerSignalEnvVar(origin, mannerRpmEstimateEnvVarSuffix), e.g.
// "Raw_Data_0_estimatedRevolutions", so a notification's Expression can
// reference the right engine's estimate by name.
const mannerRpmEstimateEnvVarSuffix = "estimatedRevolutions"

func (m *CanBusMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	frame := can.Frame{}
	if err := frame.UnmarshalJSON(r.Value); err != nil {
		return nil, err
	}

	id := dbc.MessageID(frame.ID).ToCAN()
	if m.config.IsJ1939 {
		id = getPGN(id)
	}
	if mappings, ok := m.dbc[id]; ok {
		for _, signalDef := range mappings.Signals {
			signal := extractSignal(signalDef, string(mappings.Name), frame)
			if !signal.valid {
				continue
			}

			m.env[mannerSignalEnvVar(signal.origin, signal.name)] = signal.value
			if signal.name == mannerTorqueSumSignalName {
				m.updatePeakToPeak(signal.origin, signal.value, r.Timestamp)
			}

			if mapping, ok := m.canbusMappings[signal.origin][signal.name]; ok {
				if slices.Contains(mapping.ExcludeValues, signal.value) {
					continue
				}
				m.env["value"] = signal.value
				output, err := runExpr(m.env, &mapping.MappingConfig)
				if err == nil {
					u.AddValue(message.NewValue().WithPath(mapping.Path).WithValue(output))
				} else {
					logger.GetLogger().Error(
						"Could not map value",
						zap.String("path", mapping.Path),
						zap.String("error", err.Error()),
					)
				}
			}
		}
	}
	if len(u.Values) > 0 {
		result.AddUpdate(u)
	}

	m.notifications.evaluate(m.env, r.Timestamp, result)

	return result, nil
}

// mannerSignalEnvVar names the persistent env var a decoded signal is kept
// under.
func mannerSignalEnvVar(origin, name string) string {
	return origin + "_" + name
}

// updatePeakToPeak feeds value into origin's own peakToPeakRpm (creating it
// on first use) and publishes the resulting estimate into env under
// mannerSignalEnvVar(origin, mannerRpmEstimateEnvVarSuffix).
func (m *CanBusMapper) updatePeakToPeak(origin string, value float64, now time.Time) {
	pp, ok := m.peakToPeaks[origin]
	if !ok {
		pp = &peakToPeakRpm{}
		m.peakToPeaks[origin] = pp
	}
	m.env[mannerSignalEnvVar(origin, mannerRpmEstimateEnvVarSuffix)] = pp.update(value, now)
}

func extractSignal(signalDef dbc.SignalDef, origin string, frame can.Frame) signal {
	s := descriptor.Signal{
		Start:       uint8(signalDef.StartBit),
		Length:      uint8(signalDef.Size),
		IsBigEndian: signalDef.IsBigEndian,
		IsSigned:    signalDef.IsSigned,
		IsFloat:     true,
		Scale:       signalDef.Factor,
		Offset:      signalDef.Offset,
		Min:         signalDef.Minimum,
		Max:         signalDef.Maximum,
	}

	value := s.UnmarshalPhysical(frame.Data)
	return signal{
		origin: origin,
		name:   string(signalDef.Name),
		value:  value,
		valid:  value >= s.Min && value <= s.Max,
	}
}

func readDBC(filename string, isJ1939 bool) DBC {
	file, err := os.Open(filename)
	if err != nil {
		logger.GetLogger().Error(err.Error())
	}
	defer file.Close()
	source, err := io.ReadAll(file)
	if err != nil {
		logger.GetLogger().Error(err.Error())
	}
	parser := dbc.NewParser(file.Name(), source)
	parser.Parse()
	messages := make(DBC)
	for _, def := range parser.Defs() {
		switch def := def.(type) {
		case *dbc.MessageDef:
			id := def.MessageID.ToCAN()
			if isJ1939 {
				id = getPGN(id)
			}
			messages[id] = def
		}
	}
	return messages
}

func getPGN(messageId uint32) uint32 {
	return messageId & 0x3FFFF00
}
