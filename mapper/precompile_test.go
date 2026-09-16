package mapper_test

import (
	"testing"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/mapper"
)

// Every mapper compiles its expressions when it is constructed, not on the
// first message. The constructors are what these assert against, because
// the failure being guarded here is invisible from the outside: runExpr
// caches a compiled program on whatever *config.MappingConfig it is
// handed, and a mapper that hands it a loop copy silently recompiles every
// expression for every message instead.

func mapping(expression string) config.MappingConfig {
	return config.MappingConfig{Expression: expression, Path: "propulsion.mainEngine.drive.power"}
}

func TestJSONMapperPrecompiles(t *testing.T) {
	mappings := []config.JSONMappingConfig{
		{MappingConfig: mapping("json['pwr']")},
		{MappingConfig: config.MappingConfig{
			Expression:          "json['pwr']",
			TimestampExpression: "json['ts']",
			Path:                "propulsion.mainEngine.drive.torque",
		}},
	}

	if _, err := mapper.NewJSONMapper(config.MapperConfig{Context: "testingContext"}, mappings); err != nil {
		t.Fatalf("could not construct the mapper: %v", err)
	}

	for i, m := range mappings {
		if m.CompiledExpression == nil {
			t.Errorf("mapping %d was not compiled by the constructor", i)
		}
	}
	if mappings[1].CompiledTimestampExpression == nil {
		t.Error("the timestamp expression was not compiled by the constructor")
	}
}

func TestCSVMapperPrecompiles(t *testing.T) {
	mappings := []config.CSVMappingConfig{{MappingConfig: mapping("stringValues[0]")}}

	if _, err := mapper.NewCSVMapper(config.CSVMapperConfig{}, mappings); err != nil {
		t.Fatalf("could not construct the mapper: %v", err)
	}
	if mappings[0].CompiledExpression == nil {
		t.Error("the mapping was not compiled by the constructor")
	}
}

func TestModbusMapperPrecompiles(t *testing.T) {
	mappings := []config.ModbusMappingsConfig{{MappingConfig: mapping("registers[0]")}}

	if _, err := mapper.NewModbusMapper(config.MapperConfig{Context: "testingContext"}, mappings); err != nil {
		t.Fatalf("could not construct the mapper: %v", err)
	}
	if mappings[0].CompiledExpression == nil {
		t.Error("the mapping was not compiled by the constructor")
	}
}

func TestRawModbusMapperPrecompiles(t *testing.T) {
	mappings := []config.ModbusMappingsConfig{{MappingConfig: mapping("[1,2]")}}

	if _, err := mapper.NewModbusRawMapper(config.MapperConfig{Context: "testingContext"}, mappings); err != nil {
		t.Fatalf("could not construct the mapper: %v", err)
	}
	// The constructor copies these into a map, so compilation has to
	// happen before that copy, not after.
	if mappings[0].CompiledExpression == nil {
		t.Error("the mapping was not compiled by the constructor")
	}
}

func TestBinaryMapperPrecompiles(t *testing.T) {
	mappings := []config.MappingConfig{mapping("value")}

	if _, err := mapper.NewBinaryMapper(config.MapperConfig{Context: "testingContext"}, mappings, nil); err != nil {
		t.Fatalf("could not construct the mapper: %v", err)
	}
	if mappings[0].CompiledExpression == nil {
		t.Error("the mapping was not compiled by the constructor")
	}
}

func TestExpressionFilterPrecompiles(t *testing.T) {
	mappings := []*config.ExpressionMappingConfig{{
		MappingConfig: mapping("propulsion_mainEngine_drive_power.Value > 0"),
		SourcePaths:   []string{"propulsion.mainEngine.drive.power"},
	}}

	if _, err := mapper.NewExpressionFilter(mappings); err != nil {
		t.Fatalf("could not construct the filter: %v", err)
	}
	if mappings[0].CompiledExpression == nil {
		t.Error("the mapping was not compiled by the constructor")
	}
}

func TestAggregateMapperPrecompiles(t *testing.T) {
	mappings := []*config.ExpressionMappingConfig{{
		MappingConfig: mapping("propulsion_mainEngine_drive_power.Value * 2"),
		SourcePaths:   []string{"propulsion.mainEngine.drive.power"},
	}}

	if _, err := mapper.NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings); err != nil {
		t.Fatalf("could not construct the mapper: %v", err)
	}
	if mappings[0].CompiledExpression == nil {
		t.Error("the mapping was not compiled by the constructor")
	}
}

func TestNotificationMapperPrecompilesBothExpressions(t *testing.T) {
	mappings := []*config.NotificationMappingConfig{{
		MappingConfig: config.MappingConfig{
			Expression: "propulsion_mainEngine_drive_power.Value > 100",
			Path:       "notifications.propulsion.mainEngine.drive.power",
		},
		SourcePaths: []string{"propulsion.mainEngine.drive.power"},
		When:        "propulsion_mainEngine_drive_power.Value > 0",
		Message:     "too much power",
		State:       "alarm",
	}}

	if _, err := mapper.NewNotificationMapper(config.MapperConfig{Context: "testingContext"}, mappings); err != nil {
		t.Fatalf("could not construct the mapper: %v", err)
	}
	if mappings[0].CompiledExpression == nil {
		t.Error("the mapping expression was not compiled by the constructor")
	}
	if mappings[0].CompiledWhen == nil {
		t.Error("the when expression was not compiled by the constructor")
	}
}
