package mapper

import (
	"encoding/json"
	"fmt"
	"time"

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
	// By index: precompileMapping has to write into the slice's own
	// elements, so that the copies DoMap's range loop takes later carry
	// the compiled program. See precompileMapping.
	for i := range jmc {
		precompileMapping(&jmc[i].MappingConfig)
	}
	return &JSONMapper{config: c, protocol: config.JSONType, jsonMappingConfig: jmc}, nil
}

// MapConnectorStatus implements ConnectorStatusMapper - see its doc
// comment and process in main.go.
func (m *JSONMapper) MapConnectorStatus(connector string, connected bool) *message.Mapped {
	return NewConnectorStatusUpdate(m.config.Context, connector, connected)
}

// GetTickerInterval returns the interval on which the mapper should
// additionally be re-evaluated regardless of incoming data, see
// periodicMapper in main.go. Zero disables this.
func (m *JSONMapper) GetTickerInterval() time.Duration {
	return m.config.Interval
}

func (m *JSONMapper) Map(subscriber *nanomsg.Subscriber[message.Raw], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *JSONMapper) DoMap(r *message.Raw) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(m.config.Context).WithOrigin(m.config.Context)
	s := message.NewSource().WithLabel(r.Connector).WithType(m.protocol).WithUuid(r.Uuid)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(r.Timestamp)

	// One Unmarshal per message rather than one per mapping. Every mapping
	// evaluates against the same payload, so decoding it again for each of
	// them only bought a fresh copy nothing needs - the expression
	// environment's functions are pure and none of them touch the map. A
	// mapper with three mappings therefore decoded the same bytes three
	// times: on node-hbr-rpa10 that was 28 IO-Link vibration sensors
	// publishing ~4.3 times a second, so ~360 decodes per second of a
	// 1715 byte payload where 120 will do.
	//
	// Unmarshalling here also means a message that isn't JSON at all is
	// reported once instead of once per mapping. The error is the same one
	// the loop below would have produced by mapping nothing.
	var j map[string]interface{}
	if err := json.Unmarshal(r.Value, &j); err != nil {
		logger.GetLogger().Warn(
			"Could not unmarshal the JSON message",
			zap.ByteString("JSON", r.Value),
			zap.String("Error", err.Error()),
		)
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	env := NewExpressionEnvironment()
	env["json"] = j
	// A mapping's timestampExpression used to be able to replace the
	// update's timestamp with one parsed out of the payload. It no longer
	// does: every row a mapper produces is now stamped with the time the
	// raw message arrived, so that mapped_data."time" means one thing
	// across every mapper rather than "arrival, unless this particular
	// mapping was configured otherwise".
	//
	// The guard it came with was also only half a guard. It compared
	// arrival-minus-payload against +365 days, which rejects a payload
	// timestamp a year or more in the past but accepts one arbitrarily far
	// in the future - that difference is negative, and every negative
	// number is less than 365 days. A single bad reading could put a row
	// in a chunk years ahead, where the retention policy would not reach
	// it for as long.
	//
	// config.MappingConfig.TimestampExpression is kept so that verify()
	// can warn about a configuration that still sets it, rather than
	// letting it quietly stop having an effect.
	for _, jmc := range m.jsonMappingConfig {
		output, err := runExpr(env, &jmc.MappingConfig)
		if err == nil {
			u.AddValue(message.NewValue().WithPath(jmc.Path).WithValue(output))
		}
	}

	if len(u.Values) == 0 {
		return nil, fmt.Errorf("data cannot be mapped: %v", r.Value)
	}

	return result.AddUpdate(u), nil
}
