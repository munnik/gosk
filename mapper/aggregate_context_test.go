package mapper

import (
	"testing"
	"time"

	"github.com/munnik/gosk/config"
)

func TestAggregateHistorySeparatesVessels(t *testing.T) {
	n := &config.ExpressionMappingConfig{MappingConfig: config.MappingConfig{Path: "navigation.speedOverGround.surprise", Expression: "kalman(history['navigation_speedOverGround'], 0.001, 0.01).Surprise"}, SourcePaths: []string{"navigation.speedOverGround"}, RetentionTime: 5 * time.Minute}
	m, err := NewAggregateMapper(config.MapperConfig{Context: "testingContext"}, []*config.ExpressionMappingConfig{n})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		m.DoMap(multiValueUpdate(time.Now(), "navigation.speedOverGround", 2.0))
	}
	for i := 0; i < 50; i++ {
		input := multiValueUpdate(time.Now(), "navigation.speedOverGround", 8.0).WithContext("anotherVessel")
		out, err := m.DoMap(input)
		if err != nil || out != input {
			t.Fatal("other-vessel report was not passed through unchanged")
		}
	}
	out, err := m.DoMap(multiValueUpdate(time.Now(), "navigation.speedOverGround", 2.0))
	if err != nil {
		t.Fatal(err)
	}
	if got := mustValueOf(t, out, "navigation.speedOverGround.surprise"); got != 0 {
		t.Fatalf("constant own-vessel speed raised surprise %v after other-vessel AIS reports", got)
	}
}
