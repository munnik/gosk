package mapper

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/uuid/v5"
	"go.uber.org/zap"
)

// maxTimestampSkew bounds how far a timestamp parsed out of a payload may
// sit from the time the raw message carrying it arrived, in either
// direction. A sensor whose clock is wrong by more than this is reporting
// a time that would scatter its rows into chunks nothing else is written
// to, so the arrival time is used instead and the reading is logged.
const maxTimestampSkew = 365 * 24 * time.Hour

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
func (m *JSONMapper) MapConnectorStatus(connector string, connected bool, source uuid.UUID) *message.Mapped {
	return NewConnectorStatusUpdate(m.config.Context, connector, connected, source)
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
	for _, jmc := range m.jsonMappingConfig {
		if at, ok := m.timestampFromPayload(env, &jmc, r); ok {
			u.WithTimestamp(at)
			// Keep the uuid's embedded time in step with the row's time,
			// rather than leaving it to say when the bytes arrived while
			// the row says when the measurement is for. See uuidV7At.
			u.Source.Uuid = uuidV7At(at, r.Uuid)
		}
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

// timestampFromPayload evaluates a mapping's timestampExpression against
// the decoded payload, so that a source carrying its own clock (a GPS
// fix, a sensor that stamps its readings) is recorded at the time it
// reports rather than the time its bytes reached us.
//
// The second return is false whenever the payload's answer cannot be
// trusted, in which case the caller keeps the arrival time. That covers a
// mapping with no timestampExpression at all, an expression that failed,
// one that did not return a string, one whose string is not RFC3339, and
// one whose time is implausible - see maxTimestampSkew. A rejected
// timestamp never discards the reading itself; only the time is fallen
// back on.
func (m *JSONMapper) timestampFromPayload(env ExpressionEnvironment, jmc *config.JSONMappingConfig, r *message.Raw) (time.Time, bool) {
	if jmc.TimestampExpression == "" {
		return time.Time{}, false
	}

	output, err := runTimestampExpr(env, &jmc.MappingConfig)
	if err != nil {
		return time.Time{}, false // already logged by runTimestampExpr
	}

	// Comma-ok, not a bare output.(string): a payload that simply does not
	// carry the field the expression reads evaluates to nil, and asserting
	// nil to a string panics - taking the whole mapper process down over
	// one malformed message.
	asString, ok := output.(string)
	if !ok {
		logger.GetLogger().Warn(
			"The timestamp expression did not return a string",
			zap.String("Expression", jmc.TimestampExpression),
			zap.Any("Returned", output),
			zap.String("Path", jmc.Path),
		)
		return time.Time{}, false
	}

	at, err := time.Parse(time.RFC3339, asString)
	if err != nil {
		logger.GetLogger().Warn(
			"Could not parse the returned time, please use RFC3339",
			zap.String("Error", err.Error()),
			zap.String("Returned", asString),
			zap.String("Path", jmc.Path),
		)
		return time.Time{}, false
	}

	// Checked in both directions. This used to be
	// `r.Timestamp.Sub(newTime) < 365 days`, which is one sided: a payload
	// timestamp a year or more in the past gives a large positive
	// difference and is rejected, but one arbitrarily far in the future
	// gives a negative difference, and every negative number is below any
	// positive threshold. A single bad reading could put a row in a chunk
	// years ahead, out of reach of the retention policy for that much
	// longer.
	//
	// Arrival is the reference rather than time.Now(), so that replaying
	// stored raw data checks the payload against when that data was
	// collected instead of against today.
	if skew := at.Sub(r.Timestamp); skew > maxTimestampSkew || skew < -maxTimestampSkew {
		logger.GetLogger().Warn(
			"Ignoring a timestamp from the payload that is too far from when the message arrived",
			zap.Time("Timestamp", at),
			zap.Time("Arrived", r.Timestamp),
			zap.Duration("Skew", skew),
			zap.Duration("MaxSkew", maxTimestampSkew),
			zap.String("Path", jmc.Path),
		)
		return time.Time{}, false
	}

	return at, true
}
