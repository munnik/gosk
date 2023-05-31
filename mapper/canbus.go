package mapper

import (
	"encoding/binary"
	"io/ioutil"
	"os"

	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/expression"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
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

func NewCanBusMapper(c config.CanBusMapperConfig, cmc []config.CanBusMappingConfig) (*CanBusMapper, error) {
	// parse DBC file and store mappings
	dbc := readDBC(c.DbcFile)
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

func (m *CanBusMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

func (m *CanBusMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	frm := createFrame(r)
	// lookup mappings for frame
	mappings, present := m.dbc[frm.ID]
	if present {
		// apply all mappings
		vm := vm.VM{}
		for _, mapping := range mappings.Signals {
			val := extractSignal(mapping, string(mappings.Name), frm)
			mapping, present := m.canbusMappings[val.origin][val.name]

			if present {
				env := expression.NewExpressionEnvironment()
				env["value"] = val.value
				output, err := expression.RunExpr(vm, env, mapping.ExpressionConfig)
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

	// fmt.Println(u)
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
	// fmt.Printf("[%s %f]\n", name, res)
	return Signal{origin: origin, name: string(name), value: res}
}

type Signal struct {
	origin string
	name   string
	value  float64
}
type DBC map[uint32]*dbc.MessageDef

func readDBC(filename string) DBC {
	file, err := os.Open(filename)
	if err != nil {
		logger.GetLogger().Error(err.Error())
	}
	defer file.Close()
	source, err := ioutil.ReadAll(file)
	if err != nil {
		logger.GetLogger().Error(err.Error())
	}
	parser := dbc.NewParser(file.Name(), source)
	parser.Parse()
	messages := make(map[uint32]*dbc.MessageDef)
	for _, def := range parser.Defs() {
		switch def := def.(type) {
		case *dbc.MessageDef:
			id := def.MessageID
			messages[uint32(id)] = def
		}
	}
	return messages
}
