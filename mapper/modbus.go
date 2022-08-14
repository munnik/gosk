package mapper

import (
	"encoding/binary"
	"fmt"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type ModbusMapper struct {
	config               config.MapperConfig
	protocol             string
	modbusMappingsConfig []config.ModbusMappingsConfig
}

func NewModbusMapper(c config.MapperConfig, mmc []config.ModbusMappingsConfig) (*ModbusMapper, error) {
	return &ModbusMapper{config: c, protocol: config.ModbusType, modbusMappingsConfig: mmc}, nil
}

func (m *ModbusMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

func (m *ModbusMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	if len(r.Value) < 8 {
		return nil, fmt.Errorf("no usefull data in %v", r.Value)
	}
	slave := uint8(r.Value[0])
	functionCode := binary.BigEndian.Uint16(r.Value[1:3])
	address := binary.BigEndian.Uint16(r.Value[3:5])
	numberOfCoilsOrRegisters := binary.BigEndian.Uint16(r.Value[5:7])
	registerData := make([]uint16, (len(r.Value)-7)/2)
	for i := range registerData {
		registerData[i] = binary.BigEndian.Uint16(r.Value[7+i*2 : 9+i*2])
	}
	var env = map[string]interface{}{}
	if functionCode == config.Coils || functionCode == config.DiscreteInputs {
		coilsMap := make(map[int]bool, 0)
		for i, coil := range RegistersToCoils(registerData, numberOfCoilsOrRegisters) {
			coilsMap[int(address)+i] = coil
		}
		env["coils"] = coilsMap
	} else if functionCode == config.HoldingRegisters || functionCode == config.InputRegisters {
		registersMap := make(map[int]uint16, 0)
		for i, register := range registerData {
			registersMap[int(address)+i] = register
		}
		env["registers"] = registersMap
	}

	// Reuse this vm instance between runs
	vm := vm.VM{}

	for _, mmc := range m.modbusMappingsConfig {
		if mmc.Slave != slave || mmc.FunctionCode != functionCode {
			continue
		}
		if mmc.Address < address || mmc.Address+mmc.NumberOfCoilsOrRegisters > address+numberOfCoilsOrRegisters {
			continue
		}

		// the raw message contains data that can be mapped with this register mapping
		if mmc.CompiledExpression == nil {
			// TODO: each iteration the CompiledExpression is nil
			var err error
			if mmc.CompiledExpression, err = expr.Compile(mmc.Expression, expr.Env(env)); err != nil {
				logger.GetLogger().Warn(
					"Could not compile the mapping expression",
					zap.String("Expression", mmc.Expression),
					zap.String("Error", err.Error()),
				)
				continue
			}
		}

		// the compiled program exists, let's run it
		output, err := vm.Run(mmc.CompiledExpression, env)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not run the mapping expression",
				zap.String("Expression", mmc.Expression),
				zap.String("Environment", fmt.Sprintf("%+v", env)),
				zap.String("Error", err.Error()),
			)
			continue
		}

		// the value is a map so we could try to decode it
		if m, ok := output.(map[string]interface{}); ok {
			if decoded, err := message.Decode(m); err == nil {
				output = decoded
			}
		}
		u.AddValue(message.NewValue().WithPath(mmc.Path).WithValue(output))
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}

func RegistersToCoils(registers []uint16, numberOfCoils uint16) []bool {
	result := make([]bool, 0, len(registers)*16)
	for _, r := range registers {
		result = append(result,
			r&32768 == 32768,
			r&16384 == 16384,
			r&8192 == 8192,
			r&4096 == 4096,
			r&2048 == 2048,
			r&1024 == 1024,
			r&512 == 512,
			r&256 == 256,
			r&128 == 128,
			r&64 == 64,
			r&32 == 32,
			r&16 == 16,
			r&8 == 8,
			r&4 == 4,
			r&2 == 2,
			r&1 == 1,
		)
	}
	return result[:int(numberOfCoils)]
}
