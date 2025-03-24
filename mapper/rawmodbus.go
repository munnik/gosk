package mapper

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/protocol"
)

type RawModbusMapper struct {
	config         config.MapperConfig
	protocol       string
	env            ExpressionEnvironment
	modbusMappings map[string][]config.ModbusMappingsConfig
}

func NewModbusRawMapper(c config.MapperConfig, mmc []config.ModbusMappingsConfig) (*RawModbusMapper, error) {

	mappings := make(map[string][]config.ModbusMappingsConfig)
	for _, m := range mmc {
		s := m.Path
		mappings[s] = append(mappings[s], m)
	}
	return &RawModbusMapper{
		config:         c,
		protocol:       config.ModbusType,
		modbusMappings: mappings,
		env:            NewExpressionEnvironment(),
	}, nil
}

func (m *RawModbusMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Raw]) {
	processRaw(subscriber, publisher, m)
}
func (m *RawModbusMapper) DoMap(r *message.Mapped) (*message.Raw, error) {
	result := message.NewRaw().WithType(config.ModbusType).WithConnector("ModbusReverseMapper")

	for _, svm := range r.ToSingleValueMapped() {
		if mappings, ok := m.modbusMappings[svm.Path]; ok {
			path := strings.ReplaceAll(svm.Path, ".", "_")

			m.env[path] = svm
			for _, mapping := range mappings {
				output, err := runExpr(m.env, &mapping.MappingConfig)
				if err == nil {
					array, ok := output.([]interface{})
					if !ok {
						return nil, fmt.Errorf("expression should return an array of register values")
					}
					if len(array) != int(mapping.NumberOfCoilsOrRegisters) {
						return nil, fmt.Errorf("array returned by expression should have the declared length. expected: %d, actual: %d", mapping.NumberOfCoilsOrRegisters, len(array))
					}
					registers := make([]int, len(array))
					for i, v := range array {
						value := v.(int)
						registers[i] = value
					}
					bytes := make([]byte, 0, len(array)*2)
					for _, v := range registers {
						if v > math.MaxUint16 || v < 0 {
							return nil, fmt.Errorf("register value out of range. got: %d", v)
						} else {
							uv := uint16(v)
							bytes = binary.BigEndian.AppendUint16(bytes, uv)
						}
					}

					result.WithValue(protocol.InjectModbusHeader(&mapping.ModbusHeader, bytes))

				}
			}
			return result, nil
		}

	}
	return nil, nil
}
