package mapper

import (
	"encoding/json"
	"fmt"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
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

func (m *JSONMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *JSONMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	env := NewExpressionEnvironment()

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

		env["json"] = j
		output, err := runExpr(env, &jmc.MappingConfig)
		if err == nil { // don't insert a path twice
			if v := u.GetValueByPath(jmc.Path); v != nil {
				v.WithValue(output)
			} else {
				u.AddValue(message.NewValue().WithPath(jmc.Path).WithValue(output))
			}
		}
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}
