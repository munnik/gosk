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

type JSONMapper struct {
	config            config.MapperConfig
	protocol          string
	jsonMappingConfig []config.JSONMappingConfig
}

func NewJSONMapper(c config.MapperConfig, jmc []config.JSONMappingConfig) (*JSONMapper, error) {
	return &JSONMapper{config: c, protocol: config.JSONType, jsonMappingConfig: jmc}, nil
}

func (m *JSONMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	process(subscriber, publisher, m)
}

func (m *JSONMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Collector).WithType(m.protocol)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	// Reuse this vm instance between runs
	vm := vm.VM{}

	for _, jmc := range m.jsonMappingConfig {
		var j map[string]interface{}
		if err := json.Unmarshal(r.Value, &j); err != nil {
			logger.GetLogger().Warn(
				"Could not unmarshal the JSON message",
				zap.ByteString("JSON", r.Value),
				zap.String("Error", err.Error()),
			)
			continue
		}
		env := make(map[string]interface{})
		env["json"] = j

		if jmc.CompiledExpression == nil {
			// TODO: each iteration the CompiledExpression is nil
			var err error
			if jmc.CompiledExpression, err = expr.Compile(jmc.Expression, expr.Env(env)); err != nil {
				logger.GetLogger().Warn(
					"Could not compile the mapping expression",
					zap.String("Expression", jmc.Expression),
					zap.String("Error", err.Error()),
				)
				continue
			}
		}

		// the compiled program exists, let's run it
		output, err := vm.Run(jmc.CompiledExpression, env)
		if err != nil {
			logger.GetLogger().Warn(
				"Could not run the mapping expression",
				zap.String("Expression", jmc.Expression),
				zap.String("Environment", fmt.Sprintf("%+v", env)),
				zap.String("Error", err.Error()),
			)
			continue
		}
		u.AddValue(message.NewValue().WithUuid(r.Uuid).WithPath(jmc.Path).WithValue(output))
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}
