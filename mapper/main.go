package mapper

import (
	"encoding/json"
	"fmt"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

// Mapper interface
type Mapper interface {
	Map(subscriber mangos.Socket, publisher mangos.Socket)
}

type RealMapper interface {
	DoMap(*message.Raw) (*message.Mapped, error)
}

type MappedMapper interface {
	DoMap(*message.Mapped) (*message.Mapped, error)
}

func process(subscriber mangos.Socket, publisher mangos.Socket, mapper RealMapper) {
	raw := &message.Raw{}
	var mapped *message.Mapped
	var bytes []byte
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, raw); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if mapped, err = mapper.DoMap(raw); err != nil {
			logger.GetLogger().Warn(
				"Could not map the received data",
				zap.ByteString("Raw bytes", raw.Value),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if bytes, err = json.Marshal(mapped); err != nil {
			logger.GetLogger().Warn(
				"Could not marshal the mapped data",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := publisher.Send(bytes); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
	}
}
func processMapped(subscriber mangos.Socket, publisher mangos.Socket, mapper MappedMapper) {
	in := &message.Mapped{}
	var out *message.Mapped
	var bytes []byte
	for {
		received, err := subscriber.Recv()
		if err != nil {
			logger.GetLogger().Warn(
				"Could not receive a message from the publisher",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := json.Unmarshal(received, in); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the received data",
				zap.ByteString("Received", received),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if out, err = mapper.DoMap(in); err != nil {
			logger.GetLogger().Warn(
				"Could not map the received data",
				// zap.String("input data", in),
				zap.String("Error", err.Error()),
			)
			continue
		}
		if bytes, err = json.Marshal(out); err != nil {
			logger.GetLogger().Warn(
				"Could not marshal the mapped data",
				zap.String("Error", err.Error()),
			)
			continue
		}
		if err := publisher.Send(bytes); err != nil {
			logger.GetLogger().Warn(
				"Unable to send the message using NanoMSG",
				zap.ByteString("Message", bytes),
				zap.String("Error", err.Error()),
			)
			continue
		}
	}
}

func runExpr(vm vm.VM, env map[string]interface{}, mappingConfig config.MappingConfig) (interface{}, error) {
	env, err := mergeEnvironments(env, mappingConfig.ExpressionEnvironment)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not merge the environments",
			zap.String("Error", err.Error()),
		)
		return nil, err
	}

	if mappingConfig.CompiledExpression == nil {
		// TODO: each iteration the CompiledExpression is nil
		var err error
		if mappingConfig.CompiledExpression, err = expr.Compile(mappingConfig.Expression, expr.Env(env)); err != nil {
			logger.GetLogger().Warn(
				"Could not compile the mapping expression",
				zap.String("Expression", mappingConfig.Expression),
				zap.String("Error", err.Error()),
			)
			return nil, err
		}
	}
	// the compiled program exists, let's run it
	output, err := vm.Run(mappingConfig.CompiledExpression, env)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not run the mapping expression",
			zap.String("Expression", mappingConfig.Expression),
			zap.String("Environment", fmt.Sprintf("%+v", env)),
			zap.String("Error", err.Error()),
		)
		return nil, err
	}

	// the value is a map so we could try to decode it
	if m, ok := output.(map[string]interface{}); ok {
		if decoded, err := message.Decode(m); err == nil {
			output = decoded
		}
	}

	return output, nil
}

func mergeEnvironments(left map[string]interface{}, right map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for k, v := range left {
		result[k] = v
	}
	for k, v := range right {
		if _, ok := left[k]; ok {
			return map[string]interface{}{}, fmt.Errorf("Could not merge right into left because left already contains the key %s", k)
		}
		result[k] = v
	}
	return result, nil
}

func swapPointAndComma(input string) string {
	result := []rune(input)

	for i := range result {
		if result[i] == '.' {
			result[i] = ','
		} else if result[i] == ',' {
			result[i] = '.'
		}
	}
	return string(result)
}
