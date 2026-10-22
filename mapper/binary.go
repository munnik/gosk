package mapper

import (
	"fmt"
	"time"

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
	for i := range mc {
		precompileMapping(&mc[i])
	}
	return &BinaryMapper{
		config:         c,
		protocol:       config.BinaryType,
		mappingsConfig: mc,
		env:            NewExpressionEnvironment(),
	}, nil
}

// MapConnectorStatus implements ConnectorStatusMapper - see its doc
// comment and process in main.go.
func (m *BinaryMapper) MapConnectorStatus(connector string, connected bool) *message.Mapped {
	return NewConnectorStatusUpdate(m.config.Context, connector, connected)
}

// GetTickerInterval returns the interval on which the mapper should
// additionally be re-evaluated regardless of incoming data, see
// periodicMapper in main.go. Zero disables this.
func (m *BinaryMapper) GetTickerInterval() time.Duration {
	return m.config.Interval
}

func (m *BinaryMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *BinaryMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)
	m.env["value"] = r.Value
	// By index rather than by value: &mc would point at the loop's copy,
	// so runExpr's compile cache would be written to something discarded
	// at the end of the iteration. This used to be worked around by
	// writing the copy back with m.mappingsConfig[i] = mc, which only
	// took effect for a mapping that had already produced a value once.
	// NewBinaryMapper now compiles them all up front, so there is nothing
	// left to write back.
	for i := range m.mappingsConfig {
		mc := &m.mappingsConfig[i]
		output, err := runExpr(m.env, mc)
		if err == nil {
			u.AddValue(message.NewValue().WithPath(mc.Path).WithValue(output))
		}
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}
