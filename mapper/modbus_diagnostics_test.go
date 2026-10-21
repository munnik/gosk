package mapper

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/protocol"
)

// rawReply is a modbus reply as a connector publishes it: slave, function
// code, start address and count, then the registers.
func rawReply(t time.Time, slave uint8, address uint16, registers ...uint16) *message.Raw {
	b := []byte{slave}
	b = binary.BigEndian.AppendUint16(b, protocol.ReadHoldingRegisters)
	b = binary.BigEndian.AppendUint16(b, address)
	b = binary.BigEndian.AppendUint16(b, uint16(len(registers)))
	for _, r := range registers {
		b = binary.BigEndian.AppendUint16(b, r)
	}
	r := message.NewRaw().WithConnector("connectAmpero").WithType(config.ModbusType).WithValue(b)
	r.Uuid = uuid.Nil
	r.Timestamp = t
	return r
}

func holdingRegister(address uint16, path, expression string, diagnostics map[string]string) config.ModbusMappingsConfig {
	return config.ModbusMappingsConfig{
		MappingConfig: config.MappingConfig{Path: path, Expression: expression, Diagnostics: diagnostics},
		ModbusHeader:  protocol.ModbusHeader{Slave: 1, FunctionCode: protocol.ReadHoldingRegisters, Address: address, NumberOfCoilsOrRegisters: 1},
	}
}

// The shape of an Ampero 4-20mA tank input: the level from register 52 in
// µA, the loop current and the module's "input connected" bit (register
// 50, polled on its own) as diagnostics.
func amperoTankMapper(t *testing.T, protocolOptions map[string]string) *ModbusMapper {
	t.Helper()
	m, err := NewModbusMapper(
		config.MapperConfig{Context: "testingContext", ProtocolOptions: protocolOptions},
		[]config.ModbusMappingsConfig{
			holdingRegister(52, "tanks.fuel.aft.currentLevel", "currentToRatio(registers[52] * 1.0)", map[string]string{
				"current":   "registers[52] * 1.0 / 1000000",
				"connected": "isBitSet(registers[50], 0)",
			}),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// mapOne maps one reply and returns the single value it must produce.
func mapOne(t *testing.T, m *ModbusMapper, r *message.Raw) message.Value {
	t.Helper()
	out, err := m.DoMap(r)
	if err != nil {
		t.Fatalf("DoMap: %v", err)
	}
	if len(out.Updates) != 1 || len(out.Updates[0].Values) != 1 {
		t.Fatalf("want one value, got %+v", out)
	}
	return out.Updates[0].Values[0]
}

func TestModbusAttachesDiagnostics(t *testing.T) {
	m := amperoTankMapper(t, nil)
	now := time.Now()
	// register 50 arrives in a poll of its own, which no mapping covers
	if _, err := m.DoMap(rawReply(now, 1, 50, 1)); err == nil {
		t.Fatal("a reply no mapping covers should still be an error")
	}

	v := mapOne(t, m, rawReply(now.Add(time.Second), 1, 52, 12000))
	if v.Value != 0.5 {
		t.Fatalf("value %v, want 0.5", v.Value)
	}
	if v.Diagnostics["current"] != 0.012 || v.Diagnostics["connected"] != true {
		t.Fatalf("diagnostics %+v", v.Diagnostics)
	}
	if _, rejected := v.Diagnostics[message.DiagnosticRejected]; rejected {
		t.Fatalf("an accepted reply says it was rejected: %+v", v.Diagnostics)
	}
}

func TestModbusPublishesAnAllZeroReplyAsRejected(t *testing.T) {
	m := amperoTankMapper(t, nil)
	now := time.Now()
	_, _ = m.DoMap(rawReply(now, 1, 50, 1))

	v := mapOne(t, m, rawReply(now.Add(time.Second), 1, 52, 0))
	if v.Path != "tanks.fuel.aft.currentLevel" || v.Value != nil {
		t.Fatalf("want the tank path with no value, got %+v", v)
	}
	want := map[string]any{message.DiagnosticRejected: "all zeros", "current": 0.0, "connected": true}
	for k, w := range want {
		if v.Diagnostics[k] != w {
			t.Fatalf("diagnostics %+v, want %+v", v.Diagnostics, want)
		}
	}
}

func TestModbusNamesTheErrorMarker(t *testing.T) {
	m := amperoTankMapper(t, nil)
	v := mapOne(t, m, rawReply(time.Now(), 1, 52, 0x8000))
	if got := v.Diagnostics[message.DiagnosticRejected]; got != "error marker 0x8000 in register 52" {
		t.Fatalf("rejected %q", got)
	}
	if v.Value != nil {
		t.Fatalf("a rejected reply has a value: %+v", v)
	}
}

// A rejected reply must not become the previous reading of the next one:
// a pulse counter's delta would otherwise jump by the whole count.
func TestModbusRejectedReplyLeavesTheEnvironmentAlone(t *testing.T) {
	m, err := NewModbusMapper(
		config.MapperConfig{Context: "testingContext"},
		[]config.ModbusMappingsConfig{holdingRegister(52, "propulsion.mainEngine.fuel.pulses", "deltas[52]", map[string]string{"raw": "registers[52]"})},
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mapOne(t, m, rawReply(now, 1, 52, 100))

	rejected := mapOne(t, m, rawReply(now.Add(time.Second), 1, 52, 0))
	if rejected.Diagnostics["raw"] != 0.0 {
		t.Fatalf("the rejected reply's own diagnostics should see its registers: %+v", rejected.Diagnostics)
	}

	next := mapOne(t, m, rawReply(now.Add(2*time.Second), 1, 52, 105))
	if next.Value != int32(5) {
		t.Fatalf("delta after a rejected reply is %v (%T), want 5 against the last accepted reading", next.Value, next.Value)
	}
	if next.Diagnostics["raw"] != 105.0 {
		t.Fatalf("diagnostics %+v", next.Diagnostics)
	}
}

func TestModbusSkipFaultDetectionMapsZeros(t *testing.T) {
	m := amperoTankMapper(t, map[string]string{config.ProtocolOptionModbusSkipFaultDetection: "true"})
	v := mapOne(t, m, rawReply(time.Now(), 1, 52, 0))
	if _, rejected := v.Diagnostics[message.DiagnosticRejected]; rejected || v.Value == nil {
		t.Fatalf("with fault detection off a zero is a reading: %+v", v)
	}
}

func TestModbusRejectedReplyNoMappingCoversIsAnError(t *testing.T) {
	m := amperoTankMapper(t, nil)
	if out, err := m.DoMap(rawReply(time.Now(), 1, 60, 0, 0)); err == nil {
		t.Fatalf("want an error, got %+v", out)
	}
}

func TestModbusDiagnosticsLeaveOutWhatDoesNotEvaluate(t *testing.T) {
	m, err := NewModbusMapper(
		config.MapperConfig{Context: "testingContext"},
		[]config.ModbusMappingsConfig{holdingRegister(52, "a.b", "registers[52]", map[string]string{
			"infinite":  "registers[52] / 0.0",
			"missing":   "registers[50] + nothing.Value",
			"broken":    "registers[",
			"nested":    "{'a': 1}",
			"rejected":  "'configured'",
			"connected": "true",
		})},
	)
	if err != nil {
		t.Fatal(err)
	}
	v := mapOne(t, m, rawReply(time.Now(), 1, 52, 7))
	if v.Value != uint16(7) {
		t.Fatalf("a failing diagnostic cost the value: %+v", v)
	}
	if len(v.Diagnostics) != 2 || v.Diagnostics["connected"] != true || v.Diagnostics["rejected"] != "configured" {
		t.Fatalf("diagnostics %+v, want only connected and rejected", v.Diagnostics)
	}

	// on a rejected reply the mapper's own reason wins over a diagnostic of that name
	r := mapOne(t, m, rawReply(time.Now(), 1, 52, 0))
	if r.Diagnostics[message.DiagnosticRejected] != "all zeros" {
		t.Fatalf("diagnostics %+v", r.Diagnostics)
	}
}

func TestModbusWithoutDiagnosticsIsUnchanged(t *testing.T) {
	m, err := NewModbusMapper(
		config.MapperConfig{Context: "testingContext"},
		[]config.ModbusMappingsConfig{holdingRegister(52, "a.b", "registers[52]", nil)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if v := mapOne(t, m, rawReply(time.Now(), 1, 52, 7)); v.Diagnostics != nil {
		t.Fatalf("diagnostics %+v", v.Diagnostics)
	}
	if v := mapOne(t, m, rawReply(time.Now(), 1, 52, 0)); len(v.Diagnostics) != 1 || v.Diagnostics[message.DiagnosticRejected] != "all zeros" {
		t.Fatalf("diagnostics %+v", v.Diagnostics)
	}
}
