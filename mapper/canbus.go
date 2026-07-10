package mapper

import (
	"io"
	"os"
	"slices"

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
}

type signal struct {
	origin string
	name   string
	value  float64
	valid  bool
}

type DBC map[uint32]*dbc.MessageDef

func NewCanBusMapper(c config.CanBusMapperConfig, cmc []config.CanBusMappingConfig) (*CanBusMapper, error) {
	// parse DBC file and store mappings
	dbc := readDBC(c.DbcFile, c.IsJ1939)
	mappings := make(map[string]map[string]config.CanBusMappingConfig)
	for _, m := range cmc {
		_, present := mappings[m.Origin]
		if !present {
			mappings[m.Origin] = make(map[string]config.CanBusMappingConfig)
		}
		mappings[m.Origin][m.Name] = m
	}
	return &CanBusMapper{config: c, protocol: config.CanBusType, dbc: dbc, canbusMappings: mappings}, nil
}

func (m *CanBusMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

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
		env := NewExpressionEnvironment()
		for _, signalDef := range mappings.Signals {
			signal := extractSignal(signalDef, string(mappings.Name), frame)
			if !signal.valid {
				continue
			}

			if mapping, ok := m.canbusMappings[signal.origin][signal.name]; ok {
				if slices.Contains(mapping.ExcludeValues, signal.value) {
					continue
				}
				env["value"] = signal.value
				output, err := runExpr(env, &mapping.MappingConfig)
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

	return result.AddUpdate(u), nil
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
