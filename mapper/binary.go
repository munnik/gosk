package mapper

import (
	"fmt"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type BinaryMapper struct {
	config         config.MapperConfig
	protocol       string
	mappingsConfig []config.MappingConfig
	env            ExpressionEnvironment
}

func NewBinaryMapper(c config.MapperConfig, mc []config.MappingConfig) (*BinaryMapper, error) {
	return &BinaryMapper{
		config:         c,
		protocol:       config.BinaryType,
		mappingsConfig: mc,
		env:            NewExpressionEnvironment(),
	}, nil
}

func (m *BinaryMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *BinaryMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)
	m.env["value"] = r.Value
	for i, mc := range m.mappingsConfig {
		output, err := runExpr(m.env, &mc)
		if err == nil {
			u.AddValue(message.NewValue().WithPath(mc.Path).WithValue(output))
			m.mappingsConfig[i] = mc
		}
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}
