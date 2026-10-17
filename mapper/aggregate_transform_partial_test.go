package mapper

import (
	"testing"
	"time"

	"github.com/munnik/gosk/config"
)

func TestAggregatePartialTransformUpdate(t *testing.T) {
	rev, torque := revolutionsAndTorqueTransforms()
	for _, mappings := range [][]*config.ExpressionMappingConfig{{rev, torque}, {torque, rev}} {
		m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, mappings)
		if err != nil {
			t.Fatal(err)
		}
		m.DoMap(multiValueUpdate(time.Now(), "propulsion.mainEngine.drive.revolutions", -10.0, "propulsion.mainEngine.drive.torque", 5.0))
		for _, field := range []string{"torque", "torque", "revolutions", "revolutions", "torque"} {
			value := 5.0
			if field == "revolutions" {
				value = -10.0
			}
			out, err := m.DoMap(multiValueUpdate(time.Now(), "propulsion.mainEngine.drive."+field, value))
			if err != nil {
				t.Fatal(err)
			}
			if got := mustValueOf(t, out, "propulsion.mainEngine.drive.torque"); got != -5.0 {
				t.Fatalf("partial %s update: torque %v, want -5", field, got)
			}
			if got := mustValueOf(t, out, "propulsion.mainEngine.drive.revolutions"); got != 10.0 {
				t.Fatalf("partial %s update: revolutions %v, want 10", field, got)
			}
		}
	}
}
