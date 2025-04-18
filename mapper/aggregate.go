package mapper

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type AggregateMapper struct {
	config            config.MapperConfig
	protocol          string
	retentionTime     time.Duration
	aggregateMappings map[string][]*config.ExpressionMappingConfig
	env               ExpressionEnvironment
}

func NewAggregateMapper(c config.MapperConfig, emc []*config.ExpressionMappingConfig) (*AggregateMapper, error) {
	env := NewExpressionEnvironment()
	env["history"] = make(map[string][]message.SingleValueMapped, 0)
	retentionTime := 0 * time.Second
	mappings := make(map[string][]*config.ExpressionMappingConfig)
	for _, m := range emc {
		for _, s := range m.SourcePaths {
			mappings[s] = append(mappings[s], m)
		}
		if m.RetentionTime > retentionTime {
			retentionTime = m.RetentionTime
		}
	}
	return &AggregateMapper{config: c, protocol: config.SignalKType, retentionTime: retentionTime, aggregateMappings: mappings, env: env}, nil
}

func (m *AggregateMapper) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, m, false)
}

func (m *AggregateMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	s := message.NewSource().WithLabel("signalk").WithType(m.protocol).WithUuid(uuid.Nil)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Time{}) // initialize with empty timestamp instead of hidden now

	overwrites := make(map[string]struct{}, 0)

	for _, svm := range input.ToSingleValueMapped() {
		if mappings, ok := m.aggregateMappings[svm.Path]; ok {
			if svm.Timestamp.After(u.Timestamp) { // take most recent timestamp from relevant data
				u.WithTimestamp(svm.Timestamp)
			}
			u.Source.Uuid = svm.Source.Uuid // take the uuid from the message that updated this value
			path := strings.ReplaceAll(svm.Path, ".", "_")

			if _, ok := m.env["history"]; !ok {
				m.env["history"] = make(map[string][]message.SingleValueMapped, 0)
			}
			historyMap := m.env["history"].(map[string][]message.SingleValueMapped)
			if _, ok := historyMap[path]; !ok {
				historyMap[path] = make([]message.SingleValueMapped, 0)
			}

			// remove old data from buffer
			for len(historyMap[path]) > 0 && historyMap[path][0].Timestamp.Before(time.Now().Add(-m.retentionTime)) {
				historyMap[path] = historyMap[path][1:]
			}
			historyMap[path] = append(historyMap[path], svm)

			m.env[path] = svm
			for _, mapping := range mappings {
				output, err := runExpr(m.env, &mapping.MappingConfig)
				if err == nil {
					if mapping.Overwrite {
						overwrites[mapping.Path] = struct{}{}
					}
					u.AddValue(message.NewValue().WithPath(mapping.Path).WithValue(output))
				}
			}
		}
	}

	if len(u.Values) > 0 {
		return m.removeOverWrites(input, overwrites).AddUpdate(u), nil
	} else {
		return input, nil
	}
}

func (*AggregateMapper) removeOverWrites(input *message.Mapped, overwrites map[string]struct{}) *message.Mapped {
	result := message.NewMapped().WithContext(input.Context).WithOrigin(input.Origin)
	for _, update := range input.Updates {
		u := message.NewUpdate().WithSource(update.Source).WithTimestamp(update.Timestamp)
		for _, value := range update.Values {
			if _, ok := overwrites[value.Path]; !ok {
				u.AddValue(&value)
			}
		}
		if len(u.Values) > 0 {
			result.AddUpdate(u)
		}
	}
	return result
}
