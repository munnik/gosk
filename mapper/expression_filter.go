package mapper

import (
	"fmt"
	"strings"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
)

type ExpressionFilter struct {
	filterMappings map[string][]*config.ExpressionMappingConfig
	env            ExpressionEnvironment
}

func NewExpressionFilter(emc []*config.ExpressionMappingConfig) (*ExpressionFilter, error) {
	env := NewExpressionEnvironment()

	mappings := make(map[string][]*config.ExpressionMappingConfig)
	for _, m := range emc {
		for _, s := range m.SourcePaths {
			mappings[s] = append(mappings[s], m)
		}
	}

	return &ExpressionFilter{env: env, filterMappings: mappings}, nil
}

func (f *ExpressionFilter) Map(subscriber *nanomsg.Subscriber[message.Mapped], publisher *nanomsg.Publisher[message.Mapped]) {
	process(subscriber, publisher, f)
}

func (f *ExpressionFilter) DoMap(delta *message.Mapped) (*message.Mapped, error) {
	result := message.NewMapped().WithContext(delta.Context).WithOrigin(delta.Origin)

	for _, svm := range delta.ToSingleValueMapped() {
		shouldSkip := false
		if mappings, ok := f.filterMappings[svm.Path]; ok {
			path := strings.ReplaceAll(svm.Path, ".", "_")
			f.env[path] = svm
			for _, mapping := range mappings {
				output, err := runExpr(f.env, &mapping.MappingConfig)
				if err != nil {
					return nil, err
				}
				if boolOutput, ok := output.(bool); ok {
					shouldSkip = shouldSkip || boolOutput
				} else {
					return nil, fmt.Errorf("could not cast result of the expression to bool")
				}
			}
		}

		if shouldSkip {
			continue
		}

		for _, u := range svm.ToMapped().Updates {
			result.AddUpdate(&u)
		}
	}

	return result, nil
}
