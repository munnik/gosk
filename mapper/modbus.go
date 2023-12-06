package mapper

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/protocol"
	"go.nanomsg.org/mangos/v3"
)

const (
	SLAVE_ENV_PREFIX = "slave_"
)

type ModbusMapper struct {
	config               config.MapperConfig
	protocol             string
	modbusMappingsConfig []config.ModbusMappingsConfig
	env                  ExpressionEnvironment
}

func NewModbusMapper(c config.MapperConfig, mmc []config.ModbusMappingsConfig) (*ModbusMapper, error) {
	return &ModbusMapper{
		config:               c,
		protocol:             config.ModbusType,
		modbusMappingsConfig: mmc,
		env:                  NewExpressionEnvironment(),
	}, nil
}

func (m *ModbusMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

func (m *ModbusMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	if len(r.Value) <= protocol.MODBUS_HEADER_LENGTH {
		return nil, fmt.Errorf("no useful data in %v", r.Value)
	}
	slave := uint8(r.Value[0])

	m.loadEnvironmentForSlave(slave)

	functionCode := binary.BigEndian.Uint16(r.Value[1:3])
	address := binary.BigEndian.Uint16(r.Value[3:5])
	numberOfCoilsOrRegisters := binary.BigEndian.Uint16(r.Value[5:7])
	registerData := make([]uint16, (len(r.Value)-7)/2)
	for i := range registerData {
		registerData[i] = binary.BigEndian.Uint16(r.Value[7+i*2 : 9+i*2])
	}
	if functionCode == protocol.READ_COILS || functionCode == protocol.READ_DISCRETE_INPUTS {
		coilsMap := make(map[int]bool, 0)
		for i, coil := range protocol.RegistersToCoils(registerData) {
			coilsMap[int(address)+i] = coil
		}
		m.env["coils"] = coilsMap
	} else if functionCode == protocol.READ_HOLDING_REGISTERS || functionCode == protocol.READ_INPUT_REGISTERS {
		allZero := true
		for i := range registerData {
			if registerData[i] == 0x7fff || registerData[i] == 0x8000 {
				return nil, fmt.Errorf("data %v seems to be an error message, ignoring", r.Value)
			}
			if registerData[i] != 0 {
				allZero = false
			}
		}
		if allZero {
			return nil, fmt.Errorf("data %v seems to be all zeros, ignoring", r.Value)
		}
		deltaMap := make(map[int]int32, 0)
		timestampMap := make(map[int]time.Time, 0)
		timeDeltaMap := make(map[int]int64, 0)
		if previousMap, ok := m.env["registers"].(map[int]uint16); ok {
			oldTimestampMap := m.env["timestamps"].(map[int]time.Time)
			for i, register := range registerData {
				delta := int32(register) - int32(previousMap[int(address)+i])
				if delta < -50000 { // overflow
					delta = delta + math.MaxUint16
				} else if delta > 50000 { // underflow
					delta = delta - math.MaxUint16
				}
				deltaMap[int(address)+i] = delta

				timeDeltaMap[int(address)+i] = time.Duration(r.Timestamp.Sub(oldTimestampMap[int(address)+i])).Milliseconds()
			}
		} else { // first time, no previous data available yet
			for i, register := range registerData {
				deltaMap[int(address)+i] = int32(register)
			}
		}
		registersMap := make(map[int]uint16, 0)
		for i, register := range registerData {
			registersMap[int(address)+i] = register
			timestampMap[int(address)+i] = r.Timestamp
		}
		m.env["deltas"] = deltaMap
		m.env["registers"] = registersMap
		m.env["timestamps"] = timestampMap
		m.env["timedeltas"] = timeDeltaMap
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
		output, err := runExpr(vm, m.env, mmc.MappingConfig)
		if err == nil { // don't insert a path twice
			if v := u.GetValueByPath(mmc.Path); v != nil {
				v.WithValue(output)
			} else {
				u.AddValue(message.NewValue().WithPath(mmc.Path).WithValue(output))
			}
		}
	}

	m.writeEnvironmentForSlave(slave)

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}

func (m *ModbusMapper) loadEnvironmentForSlave(slave uint8) {
	slaveString := fmt.Sprintf(SLAVE_ENV_PREFIX+"%d", slave)
	if slaveEnvironment, ok := m.env[slaveString]; ok {
		for k, v := range slaveEnvironment.(ExpressionEnvironment) {
			m.env[k] = v
		}
	}
}

func (m *ModbusMapper) writeEnvironmentForSlave(slave uint8) {
	slaveString := fmt.Sprintf(SLAVE_ENV_PREFIX+"%d", slave)
	for k, v := range m.env {
		if strings.HasPrefix(k, SLAVE_ENV_PREFIX) {
			continue
		}
		if _, ok := m.env[slaveString]; !ok {
			m.env[slaveString] = ExpressionEnvironment{}
		}
		m.env[slaveString].(ExpressionEnvironment)[k] = v
	}
}
