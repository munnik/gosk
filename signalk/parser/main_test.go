package parser_test

import (
	"testing"

	"github.com/munnik/gosk/signalk/parser"
)

func TestWithUnknownDataTypeDeltaFromData(t *testing.T) {
	const dataType = "Unknown"
	delta, err := parser.DeltaFromData([]byte{}, dataType)
	if err == nil {
		t.Errorf("Expected an error for %s data type", dataType)
	}
	if !delta.IsEmpty() {
		t.Errorf("Expected an empty delta but got %v instead", delta)
	}
}
