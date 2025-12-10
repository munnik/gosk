package mapper

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"

	"github.com/brutella/can"
	"go.einride.tech/can/pkg/dbc"
)

type CanBusMapper struct {
	config         config.CanBusMapperConfig
	protocol       string
	dbc            DBC
	canbusMappings map[string]map[string]config.CanBusMappingConfig
}

type Signal struct {
	origin string
	name   string
	value  float64
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

	frm := createFrame(r)
	// lookup mappings for frame
	id := frm.ID
	if m.config.IsJ1939 {
		id = (id >> 8) & 0x3FFFF
	}
	mappings, present := m.dbc[id]
	if present {
		// apply all mappings
		fmt.Printf("Found %s with id %d\n", mappings.Name, mappings.MessageID)
		env := NewExpressionEnvironment()
		for _, mapping := range mappings.Signals {
			val := extractSignal(mapping, string(mappings.Name), frm)
			mapping, present := m.canbusMappings[val.origin][val.name]

			if present {
				env["value"] = val.value
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

func createFrame(r *message.Raw) can.Frame {
	data := [8]uint8{}
	copy(data[:], r.Value[8:16])
	frm := can.Frame{
		ID:     binary.BigEndian.Uint32(r.Value[0:4]),
		Length: r.Value[4],
		Flags:  r.Value[5],
		Res0:   r.Value[6],
		Res1:   r.Value[7],
		Data:   data,
	}
	return frm
}

func extractSignal(mapping dbc.SignalDef, origin string, frm can.Frame) Signal {
	// get name
	name := mapping.Name
	start := mapping.StartBit
	length := mapping.Size
	data := make([]uint8, 8)
	copy(data, frm.Data[:])
	if mapping.IsBigEndian {
		start = start - 7
		// reverse the bits in each byte
		// for i, b := range data {
		// 	data[i] = bits.Reverse8(b)
		// }
	}
	// extract the correct bits
	temp := binary.BigEndian.Uint64(data[:])
	temp = temp << start
	temp = temp >> (64 - (length))

	// get value
	var val float64
	if mapping.IsSigned {
		val = float64(int64(temp))
	} else {
		val = float64(temp)
	}

	// get conversion
	res := val*mapping.Factor + mapping.Offset
	return Signal{origin: origin, name: string(name), value: res}
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
			id := uint32(def.MessageID)
			if isJ1939 {
				id = (id >> 8) & 0x3FFFF
			}
			messages[id] = def
		}
	}
	return messages
}
