package mapper

import (
	"fmt"
	"time"

	"github.com/antonmedv/expr/vm"
	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"go.nanomsg.org/mangos/v3"
)

type AggegrateMapper struct {
	config            config.MapperConfig
	protocol          string
	aggegrateMappings map[string][]config.AggegrateMappingConfig
	env               map[string]interface{}
}

func NewAggegrateMapper(c config.MapperConfig, amc []config.AggegrateMappingConfig) (*AggegrateMapper, error) {
	mappings := make(map[string][]config.AggegrateMappingConfig)
	for _, m := range amc {
		for _, s := range m.SourcePaths {
			mappings[s] = append(mappings[s], m)
		}
	}
	env := make(map[string]interface{})
	return &AggegrateMapper{config: c, protocol: config.SignalKType, aggegrateMappings: mappings, env: env}, nil
}
func (m *AggegrateMapper) Map(subscriber mangos.Socket, publisher mangos.Socket) {
	processMapped(subscriber, publisher, m)
}

func (m *AggegrateMapper) DoMap(input *message.Mapped) (*message.Mapped, error) {
	s := message.NewSource().WithLabel("signalk").WithType(m.protocol).WithUuid(uuid.Nil)
	u := message.NewUpdate().WithSource(*s).WithTimestamp(time.Now())
	for _, svm := range input.ToSingleValueMapped() {
		mappings, present := m.aggegrateMappings[svm.Path]
		if present {
			fmt.Println(svm.Path)
			m.env[svm.Path] = svm
			vm := vm.VM{}
			for _, mapping := range mappings {
				output, err := runExpr(vm, m.env, mapping.MappingConfig)
				if err == nil {
					u.AddValue(message.NewValue().WithPath(mapping.Path).WithValue(output))
				}
			}

		}
	}
	return input.AddUpdate(u), nil
	// return input, nil
}
