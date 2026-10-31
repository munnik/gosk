package mapper

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"strconv"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/protocol"
)

type ModbusMapper struct {
	config               config.MapperConfig
	protocol             string
	modbusMappingsConfig []config.ModbusMappingsConfig
	env                  map[uint8]ExpressionEnvironment
}

func NewModbusMapper(c config.MapperConfig, mmc []config.ModbusMappingsConfig) (*ModbusMapper, error) {
	for i := range mmc {
		precompileMapping(&mmc[i].MappingConfig)
	}
	return &ModbusMapper{
		config:               c,
		protocol:             config.ModbusType,
		modbusMappingsConfig: mmc,
		env:                  make(map[uint8]ExpressionEnvironment),
	}, nil
}

// MapConnectorStatus implements ConnectorStatusMapper - see its doc
// comment and process in main.go.
func (m *ModbusMapper) MapConnectorStatus(connector string, connected bool) *message.Mapped {
	return NewConnectorStatusUpdate(m.config.Context, connector, connected)
}

// GetTickerInterval returns the interval on which the mapper should
// additionally be re-evaluated regardless of incoming data, see
// periodicMapper in main.go. Zero disables this.
func (m *ModbusMapper) GetTickerInterval() time.Duration {
	return m.config.Interval
}

func (m *ModbusMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *ModbusMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol)
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
		if m.env[slave]["coils"] == nil {
			m.env[slave]["coils"] = make(map[int]bool, 0)
		}
		maps.Copy(m.env[slave]["coils"].(map[int]bool), coilsMap)
	} else if functionCode == protocol.ReadHoldingRegisters || functionCode == protocol.ReadInputRegisters {
		skipFaultDetection := false
		if _, ok := m.config.ProtocolOptions[config.ProtocolOptionModbusSkipFaultDetection]; ok {
			skipFaultDetection, _ = strconv.ParseBool(m.config.ProtocolOptions[config.ProtocolOptionModbusSkipFaultDetection])
		}

		if !skipFaultDetection {
			if reason := rejectionReason(address, registerData); reason != "" {
				return m.mapRejected(r, slave, functionCode, address, numberOfCoilsOrRegisters, registerData, reason, result, u)
			}
		}

		registersMap := make(map[int]uint16, len(registerData))
		deltaMap := make(map[int]int32, len(registerData))
		timestampMap := make(map[int]time.Time, len(registerData))
		timeDeltaMap := make(map[int]int64, len(registerData))

		if previousRegisterMap, ok := m.env[slave]["registers"].(map[int]uint16); ok {
			previousTimestampMap := m.env[slave]["timestamps"].(map[int]time.Time)
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
		if _, ok := m.env[slave]["deltas"].(map[int]int32); ok {
			maps.Copy(m.env[slave]["deltas"].(map[int]int32), deltaMap)
		} else {
			m.env[slave]["deltas"] = deltaMap
		}
		if _, ok := m.env[slave]["registers"].(map[int]uint16); ok {
			maps.Copy(m.env[slave]["registers"].(map[int]uint16), registersMap)
		} else {
			m.env[slave]["registers"] = registersMap
		}
		if _, ok := m.env[slave]["timestamps"].(map[int]time.Time); ok {
			maps.Copy(m.env[slave]["timestamps"].(map[int]time.Time), timestampMap)
		} else {
			m.env[slave]["timestamps"] = timestampMap
		}
		if _, ok := m.env[slave]["timedeltas"].(map[int]int64); ok {
			maps.Copy(m.env[slave]["timedeltas"].(map[int]int64), timeDeltaMap)
		} else {
			m.env[slave]["timedeltas"] = timeDeltaMap
		}
	}

	for i := range m.modbusMappingsConfig {
		mmc := &m.modbusMappingsConfig[i]
		if !coveredBy(mmc, slave, functionCode, address, numberOfCoilsOrRegisters) {
			continue
		}
		output, err := runExpr(m.env[slave], &mmc.MappingConfig)
		if err == nil && output != nil {
			u.AddValue(message.NewValue().WithPath(mmc.Path).WithValue(output).WithDiagnostics(runDiagnostics(m.env[slave], &mmc.MappingConfig)))
		}
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}

// rejectionReason returns why a reply of holding or input registers looks
// like a device fault rather than a reading, or "" when it looks like a
// reading: a register holding 0x7FFF or 0x8000, which many devices put in
// a register they cannot fill, or a reply that is zero throughout, which is
// what an open 4-20mA loop or an unpowered module returns. These are the
// checks the mapper has always made; what changed is what it does with a
// reply that fails them (see mapRejected).
func rejectionReason(address uint16, registerData []uint16) string {
	for i, register := range registerData {
		if register == 0x7fff || register == 0x8000 {
			return fmt.Sprintf("error marker 0x%04X in register %d", register, int(address)+i)
		}
	}
	for _, register := range registerData {
		if register != 0 {
			return ""
		}
	}
	return "all zeros"
}

// mapRejected publishes a reply rejected as a suspected device fault
// instead of dropping it in silence. Every mapping the reply covers gets a
// value with no value, carrying the reason under message.DiagnosticRejected
// and its own diagnostics evaluated against the rejected registers - so an
// open loop arrives as "rejected: all zeros" with a loop current of 0 rather
// than as a sensor that simply stopped reporting, which looks the same as a
// node that is off or a link that is down.
//
// The rejected registers never enter the slave's environment. Only the
// diagnostics see them, in a copy (rejectedEnvironment): the value
// expressions of later replies keep computing from the last reply that was
// accepted, as they always have. That matters most for a pulse counter,
// whose delta against a rejected zero would be a huge jump. deltas and
// timedeltas in that copy therefore still describe the last accepted reply.
//
// A reply that no mapping covers is still an error, as before, so the
// journal says something when a rejected reply cannot be published at all.
func (m *ModbusMapper) mapRejected(r *message.Raw, slave uint8, functionCode, address, numberOfCoilsOrRegisters uint16, registerData []uint16, reason string, result *message.Mapped, u *message.Update) (*message.Mapped, error) {
	env := m.rejectedEnvironment(slave, address, registerData, r.Timestamp)
	for i := range m.modbusMappingsConfig {
		mmc := &m.modbusMappingsConfig[i]
		if !coveredBy(mmc, slave, functionCode, address, numberOfCoilsOrRegisters) {
			continue
		}
		diagnostics := runDiagnostics(env, &mmc.MappingConfig)
		if diagnostics == nil {
			diagnostics = make(map[string]any, 1)
		}
		diagnostics[message.DiagnosticRejected] = reason
		u.AddValue(message.NewValue().WithPath(mmc.Path).WithValue(nil).WithDiagnostics(diagnostics))
	}
	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data %v was rejected (%s) and no mapping covers it, ignoring", r.Value, reason)
	}
	return result.AddUpdate(u), nil
}

// rejectedEnvironment is a copy of slave's environment in which the
// registers and timestamps of a rejected reply are visible, for the
// diagnostics of that reply alone. The slave's own environment is left as
// it was.
func (m *ModbusMapper) rejectedEnvironment(slave uint8, address uint16, registerData []uint16, timestamp time.Time) ExpressionEnvironment {
	env := make(ExpressionEnvironment, len(m.env[slave]))
	maps.Copy(env, m.env[slave])

	registers := make(map[int]uint16)
	if previous, ok := m.env[slave]["registers"].(map[int]uint16); ok {
		maps.Copy(registers, previous)
	}
	timestamps := make(map[int]time.Time)
	if previous, ok := m.env[slave]["timestamps"].(map[int]time.Time); ok {
		maps.Copy(timestamps, previous)
	}
	for i, register := range registerData {
		registers[int(address)+i] = register
		timestamps[int(address)+i] = timestamp
	}
	env["registers"] = registers
	env["timestamps"] = timestamps
	return env
}

// coveredBy reports whether mmc reads only registers (or coils) of slave
// that a reply with this function code, start address and count contains.
func coveredBy(mmc *config.ModbusMappingsConfig, slave uint8, functionCode, address, numberOfCoilsOrRegisters uint16) bool {
	if mmc.Slave != slave || mmc.FunctionCode != functionCode {
		return false
	}
	return mmc.Address >= address && mmc.Address+mmc.NumberOfCoilsOrRegisters <= address+numberOfCoilsOrRegisters
}

func (m *ModbusMapper) loadEnvironmentForSlave(slave uint8) {
	if _, ok := m.env[slave]; !ok {
		m.env[slave] = NewExpressionEnvironment()
	}
}
