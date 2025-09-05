package mapper

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/protocol"
)

const (
	slaveEnvPrefix = "slave_"
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

func (m *ModbusMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
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
	if functionCode == protocol.ReadCoils || functionCode == protocol.ReadDiscreteInputs {
		coilsMap := make(map[int]bool, 0)
		for i, coil := range protocol.RegistersToCoils(registerData) {
			coilsMap[int(address)+i] = coil
		}
		maps.Copy(m.env["coils"].(map[int]bool), coilsMap)
	} else if functionCode == protocol.ReadHoldingRegisters || functionCode == protocol.ReadInputRegisters {
		skipFaultDetection := false
		if _, ok := m.config.ProtocolOptions[config.ProtocolOptionModbusSkipFaultDetection]; ok {
			skipFaultDetection, _ = strconv.ParseBool(m.config.ProtocolOptions[config.ProtocolOptionModbusSkipFaultDetection])
		}

		if !skipFaultDetection {
			// todo, fix with sensor error detection
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
		}

		registersMap := make(map[int]uint16, len(registerData))
		deltaMap := make(map[int]int32, len(registerData))
		timestampMap := make(map[int]time.Time, len(registerData))
		timeDeltaMap := make(map[int]int64, len(registerData))

		if previousRegisterMap, ok := m.env["registers"].(map[int]uint16); ok {
			previousTimestampMap := m.env["timestamps"].(map[int]time.Time)
			for i, register := range registerData {
				delta := int32(register) - int32(previousRegisterMap[int(address)+i])
				if delta < -50000 { // overflow
					delta = delta + math.MaxUint16
				} else if delta > 50000 { // underflow
					delta = delta - math.MaxUint16
				}
				deltaMap[int(address)+i] = delta
				timeDeltaMap[int(address)+i] = time.Duration(r.Timestamp.Sub(previousTimestampMap[int(address)+i])).Milliseconds()
			}
		} else { // first time, no previous data available yet
			for i, register := range registerData {
				deltaMap[int(address)+i] = int32(register)
			}
		}
		for i, register := range registerData {
			registersMap[int(address)+i] = register
			timestampMap[int(address)+i] = r.Timestamp
		}
		if _, ok := m.env["deltas"].(map[int]int32); ok {
			maps.Copy(m.env["deltas"].(map[int]int32), deltaMap)
		} else {
			m.env["deltas"] = deltaMap
		}
		if _, ok := m.env["registers"].(map[int]uint16); ok {
			maps.Copy(m.env["registers"].(map[int]uint16), registersMap)
		} else {
			m.env["registers"] = registersMap
		}
		if _, ok := m.env["timestamps"].(map[int]time.Time); ok {
			maps.Copy(m.env["timestamps"].(map[int]time.Time), timestampMap)
		} else {
			m.env["timestamps"] = timestampMap
		}
		if _, ok := m.env["timedeltas"].(map[int]int64); ok {
			maps.Copy(m.env["timedeltas"].(map[int]int64), timeDeltaMap)
		} else {
			m.env["timedeltas"] = timeDeltaMap
		}
	}

	for _, mmc := range m.modbusMappingsConfig {
		if mmc.Slave != slave || mmc.FunctionCode != functionCode {
			continue
		}
		if mmc.Address < address || mmc.Address+mmc.NumberOfCoilsOrRegisters > address+numberOfCoilsOrRegisters {
			continue
		}
		output, err := runExpr(m.env, &mmc.MappingConfig)
		if err == nil {
			u.AddValue(message.NewValue().WithPath(mmc.Path).WithValue(output))
		}
	}

	m.writeEnvironmentForSlave(slave)

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}

func (m *ModbusMapper) loadEnvironmentForSlave(slave uint8) {
	slaveString := fmt.Sprintf("slaveEnvPrefix%d", slave)
	if slaveEnvironment, ok := m.env[slaveString]; ok {
		for k, v := range slaveEnvironment.(ExpressionEnvironment) {
			m.env[k] = v
		}
	}
}

func (m *ModbusMapper) writeEnvironmentForSlave(slave uint8) {
	slaveString := fmt.Sprintf("slaveEnvPrefix%d", slave)
	for k, v := range m.env {
		if strings.HasPrefix(k, slaveEnvPrefix) {
			continue
		}
		if _, ok := m.env[slaveString]; !ok {
			m.env[slaveString] = ExpressionEnvironment{}
		}
		m.env[slaveString].(ExpressionEnvironment)[k] = v
	}
}
