package mapper

import (
	"encoding/binary"
	"fmt"

	"github.com/antonmedv/expr"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type ModbusMapper struct {
	config                config.MapperConfig
	protocol              string
	registerMappingConfig []config.RegisterMappingConfig
}

func NewModbusMapper(c config.MapperConfig, rmc []config.RegisterMappingConfig) (*ModbusMapper, error) {
	return &ModbusMapper{config: c, protocol: config.ModbusType, registerMappingConfig: rmc}, nil
}

func (m *ModbusMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

func (m *ModbusMapper) doMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Collector).WithType(m.protocol)
	u := message.NewUpdate().WithSource(s).WithTimestamp(r.Timestamp)

	if len(r.Value) < 7 {
		return nil, fmt.Errorf("no usefull data in %v", r.Value)
	}
	slave := uint8(r.Value[0])
	functionCode := binary.BigEndian.Uint16(r.Value[1:3])
	address := binary.BigEndian.Uint16(r.Value[3:5])
	numberOfRegisters := binary.BigEndian.Uint16(r.Value[5:7])
	registerData := make([]uint16, numberOfRegisters)
	if uint16(len(r.Value)) != (7 + 2*numberOfRegisters) {
		return nil, fmt.Errorf("the length of the value is not equal to %v but %v", 7+2*numberOfRegisters, len(r.Value))
	}
	for i := uint16(0); i < numberOfRegisters; i += 1 {
		registerData[i] = binary.BigEndian.Uint16(r.Value[7+i*2 : 9+i*2])
	}

	for _, rmc := range m.registerMappingConfig {
		if rmc.Slave != slave || rmc.FunctionCode != functionCode {
			continue
		}
		if rmc.Address < address || rmc.Address+rmc.NumberOfRegisters > address+numberOfRegisters {
			continue
		}

		// the raw message contains data that can be mapped with this register mapping
		var env = map[string]interface{}{
			"registers": []uint16{},
		}
		if rmc.CompiledExpression == nil {
			// TODO: each iteration the CompiledExpression is nil
			var err error
			if rmc.CompiledExpression, err = expr.Compile(rmc.Expression, expr.Env(env)); err != nil {
				logger.GetLogger().Warn(
					"Could not compile the mapping expression",
					zap.String("Expression", rmc.Expression),
					zap.String("Error", err.Error()),
				)
				continue
			}
		}

		// the compiled program exists, let's run it
		env["registers"] = registerData[rmc.Address-address : rmc.Address-address+rmc.NumberOfRegisters]
		output, err := expr.Run(rmc.CompiledExpression, env)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not run the mapping expression",
				zap.String("Expression", rmc.Expression),
				zap.String("Environment", fmt.Sprintf("%+v", env)),
				zap.String("Error", err.Error()),
			)
			continue
		}
		u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath(rmc.Path).WithValue(output))
	}

	if len(u.Values) == 0 {
		return result, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}
	return result.AddUpdate(u), nil
}
