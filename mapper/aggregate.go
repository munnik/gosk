package mapper

import (
	"fmt"
	"strings"
	"time"

	"github.com/expr-lang/expr/vm"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
)

type AggregateMapper struct {
	config            config.MapperConfig
	protocol          string
	retentionTime     time.Duration
	aggregateMappings map[string][]config.ExpressionMappingConfig
	env               ExpressionEnvironment
}

func NewAggregateMapper(c config.MapperConfig, emc []config.ExpressionMappingConfig) (*AggregateMapper, error) {
	env := NewExpressionEnvironment()
	retentionTime := 0 * time.Second
	mappings := make(map[string][]config.ExpressionMappingConfig)
	for _, m := range emc {
		for _, s := range m.SourcePaths {
			mappings[s] = append(mappings[s], m)
		}
		if m.RetentionTime > retentionTime {
			retentionTime = m.RetentionTime
		}
	}
	fmt.Println(retentionTime)
	return &AggregateMapper{config: c, protocol: config.SignalKType, retentionTime: retentionTime, aggregateMappings: mappings, env: env}, nil
}

func (m *AggregateMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	processMapped(subscriber, publisher, m)
}

func (m *AggregateMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	s := message.NewSource().WithLabel("signalk").WithType(m.protocol).WithUuid(uuid.Nil)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Time{}) // initialize with empty timestamp instead of hidden now
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
			for len(historyMap[path]) > 0 && historyMap[path][0].Timestamp.Before(time.Now().Add(-m.retentionTime)) { // remove old data from buffer
				historyMap[path] = historyMap[path][1:]

			}
			historyMap[path] = append(historyMap[path], svm)
			m.env[path] = svm
			vm := vm.VM{}
			for _, mapping := range mappings {
				output, err := runExpr(vm, m.env, mapping.MappingConfig)
				if err == nil { // don't insert a path twice
					if v := u.GetValueByPath(mapping.Path); v != nil {
						v.WithValue(output)
					} else {
						u.AddValue(message.NewValue().WithPath(mapping.Path).WithValue(output))
					}
				}
			}
		}
	}
	if len(u.Values) > 0 {
		return input.AddUpdate(u), nil
	} else {
		return input, nil
	}
}
