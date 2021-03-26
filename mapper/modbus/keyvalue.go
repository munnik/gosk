package modbus

import (
	"encoding/binary"
	"errors"

	"github.com/antonmedv/expr"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/mapper/signalk"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

var env = map[string]interface{}{
	"registers": []uint16{},
}

func KeyValueFromModbus(m *nanomsg.RawData, cfg config.ModbusConfig) ([]signalk.Value, error) {
	result := make([]signalk.Value, 0)

	var functionCode uint16
	var startRegister uint16
	var registerCount uint16
	var value nanomsg.MappedDataCreator

	functionCode = binary.BigEndian.Uint16(m.Payload[0:2])
	startRegister = binary.BigEndian.Uint16(m.Payload[2:4])
	registerCount = binary.BigEndian.Uint16(m.Payload[4:6])
	registerData := make([]uint16, registerCount)
	for i := uint16(0); i < registerCount; i += 1 {
		registerData[i] = binary.BigEndian.Uint16(m.Payload[6+i*2 : 8+i*2])
	}

	logger.GetLogger().Info(
		"Received modbus data for mapping",
		zap.String("Header", m.Header.String()),
		zap.Uint16("Function code", functionCode),
		zap.Uint16("Start register", startRegister),
		zap.Uint16("Register count", registerCount),
		zap.Uint16s("Register data", registerData),
	)

	for i := uint16(0); i < registerCount; i += 1 {
		if registerMapping, ok := cfg.RegisterMappings[i+startRegister]; ok {
			if registerMapping.CompiledExpression == nil {
				continue
			}
			env["registers"] = registerData[i : i+registerMapping.Size]
			output, err := expr.Run(registerMapping.CompiledExpression, env)
			if err != nil {
				logger.GetLogger().Warn(
					"Could not run the execute expression",
					zap.String("Mapping expression", registerMapping.Expression),
					zap.String("Error", err.Error()),
				)
				continue
			}

			value, err = nanomsg.NewMappedDataCreator(output)
			if err != nil {
				logger.GetLogger().Warn(
					"Could create a value from the result of the expression",
					zap.String("Error", err.Error()),
				)
				continue
			}

			result = append(
				result,
				signalk.Value{
					Context: cfg.Context,
					Path:    registerMapping.SignalKPath,
					Value:   value,
				},
			)
		}
	}

	if len(result) == 0 {
		return nil, errors.New("no mapping found for this modbus register range")
	}
	return result, nil
}
