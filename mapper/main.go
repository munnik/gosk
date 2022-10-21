package mapper

import (
	"encoding/json"
	"fmt"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
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
		nanomsg.SendMapped(mapped, publisher)
	}
}
func processMapped(subscriber mangos.Socket, publisher mangos.Socket, mapper MappedMapper) {
	in := &message.Mapped{}
	var out *message.Mapped
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
		nanomsg.SendMapped(out, publisher)
	}
}
func runExpr(vm vm.VM, env map[string]interface{}, mapping config.MappingConfig) (interface{}, error) {
	if mapping.CompiledExpression == nil {
		// TODO: each iteration the CompiledExpression is nil
		var err error
		if mapping.CompiledExpression, err = expr.Compile(mapping.Expression, expr.Env(env)); err != nil {
			logger.GetLogger().Warn(
				"Could not compile the mapping expression",
				zap.String("Expression", mapping.Expression),
				zap.String("Error", err.Error()),
			)
			return nil, err
		}
	}
	// the compiled program exists, let's run it
	output, err := vm.Run(mapping.CompiledExpression, env)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not run the mapping expression",
			zap.String("Expression", mapping.Expression),
			zap.String("Environment", fmt.Sprintf("%+v", env)),
			zap.String("Error", err.Error()),
		)
		return nil, err
	}
	return output, nil
}
