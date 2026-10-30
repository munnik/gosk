package mapper_test

import (
	"math"
	"testing"

	"github.com/munnik/gosk/config"
	. "github.com/munnik/gosk/mapper"
	"github.com/munnik/gosk/message"
)

// The mappings a two-sensor shaft power meter's frames are read with: the
// torque from the sensor words rather than the meter's combined word, so
// that one dead sensor (a raw word of 0, the meter's error indication)
// leaves the other's torque instead of -111 % of the range.
const (
	torqueRange = "10000.0"
	torqueOf    = " - 32768) / 32768.0 * 1.1111111 * " + torqueRange
)

var twoSensorMappings = []config.MappingConfig{
	{Expression: "(toUInt(value[6], value[7])) / 60.0", Path: "drive.revolutions"},
	{Expression: "let t = toUInt(value[8], value[9]); t == 0 ? nil : (t - 32768) / 128.0 + 273.15", Path: "drive.torque.temperature"},
	{Expression: "let s1 = toUInt(value[4], value[5]); let s2 = toUInt(value[2], value[3]); s1 == 0 && s2 == 0 ? nil : (s1 == 0 ? (s2" + torqueOf + " : (s2 == 0 ? (s1" + torqueOf + " : ((s1 + s2) / 2.0" + torqueOf + "))", Path: "drive.torque"},
	{Expression: "let s = toUInt(value[4], value[5]); s == 0 ? nil : (s" + torqueOf, Path: "drive.torque.sensor1"},
	{Expression: "let s = toUInt(value[2], value[3]); s == 0 ? nil : (s" + torqueOf, Path: "drive.torque.sensor2"},
}

// frame lays a meter frame out as the binary mapper sees it: sensor 2,
// sensor 1, rpm, temperature and the combined word, big-endian, after a
// two-byte header.
func frame(sensor2, sensor1, rpm, temperature, combined uint16) []byte {
	b := []byte{0, 0}
	for _, w := range []uint16{sensor2, sensor1, rpm, temperature, combined} {
		b = append(b, byte(w>>8), byte(w))
	}
	return b
}

func torqueFrom(word float64) float64 {
	return (word - 32768) / 32768 * 1.1111111 * 10000
}

func mapFrame(t *testing.T, value []byte) map[string]interface{} {
	t.Helper()
	m, err := NewBinaryMapper(config.MapperConfig{Context: "vessels.urn:mrn:imo:mmsi:000000000"}, twoSensorMappings)
	if err != nil {
		t.Fatal(err)
	}
	raw := message.NewRaw().WithConnector("meter").WithValue(value)
	mapped, err := m.DoMap(raw)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]interface{}{}
	for _, u := range mapped.Updates {
		for _, v := range u.Values {
			got[v.Path] = v.Value
		}
	}
	return got
}

func wantFloat(t *testing.T, got map[string]interface{}, path string, want float64) {
	t.Helper()
	v, ok := got[path]
	if !ok {
		t.Errorf("%s: no value, want %v", path, want)
		return
	}
	f, ok := v.(float64)
	if !ok || math.Abs(f-want) > 1e-6 {
		t.Errorf("%s: got %v (%T), want %v", path, v, v, want)
	}
}

func wantAbsent(t *testing.T, got map[string]interface{}, path string) {
	t.Helper()
	if v, ok := got[path]; ok {
		t.Errorf("%s: got %v, want no value", path, v)
	}
}

func TestBinaryMapperTwoSensorsAlive(t *testing.T) {
	// Both sensors at rest, slightly either side of zero; the meter's
	// combined word is their mean.
	got := mapFrame(t, frame(32396, 33460, 0, 32768+28*128, 32928))
	wantFloat(t, got, "drive.revolutions", 0)
	wantFloat(t, got, "drive.torque.temperature", 28+273.15)
	wantFloat(t, got, "drive.torque", torqueFrom(32928))
	wantFloat(t, got, "drive.torque.sensor1", torqueFrom(33460))
	wantFloat(t, got, "drive.torque.sensor2", torqueFrom(32396))
}

func TestBinaryMapperOneSensorDead(t *testing.T) {
	// Sensor 1 dead: its word, the temperature (that rotor's) and the
	// combined word are all 0. The torque is sensor 2's alone and the
	// dead sensor's own paths publish nothing.
	got := mapFrame(t, frame(32612, 0, 139*60, 0, 0))
	wantFloat(t, got, "drive.revolutions", 139)
	wantAbsent(t, got, "drive.torque.temperature")
	wantFloat(t, got, "drive.torque", torqueFrom(32612))
	wantAbsent(t, got, "drive.torque.sensor1")
	wantFloat(t, got, "drive.torque.sensor2", torqueFrom(32612))

	// And the other way round.
	got = mapFrame(t, frame(0, 32612, 0, 32768, 0))
	wantFloat(t, got, "drive.torque", torqueFrom(32612))
	wantFloat(t, got, "drive.torque.sensor1", torqueFrom(32612))
	wantAbsent(t, got, "drive.torque.sensor2")
}

func TestBinaryMapperBothSensorsDead(t *testing.T) {
	got := mapFrame(t, frame(0, 0, 0, 0, 0))
	wantFloat(t, got, "drive.revolutions", 0)
	for _, path := range []string{"drive.torque", "drive.torque.sensor1", "drive.torque.sensor2", "drive.torque.temperature"} {
		wantAbsent(t, got, path)
	}
}

// A nil result publishes nothing, rather than a null that everything
// downstream would take for a reading.
func TestBinaryMapperNilResultPublishesNothing(t *testing.T) {
	m, err := NewBinaryMapper(config.MapperConfig{Context: "vessels.urn:mrn:imo:mmsi:000000000"}, []config.MappingConfig{
		{Expression: "nil", Path: "nothing"},
		{Expression: "value[0] == 0 ? nil : 1", Path: "sometimes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if mapped, err := m.DoMap(message.NewRaw().WithConnector("meter").WithValue([]byte{0})); err == nil {
		t.Errorf("got %+v, want an error for a frame that maps to nothing", mapped)
	}
	mapped, err := m.DoMap(message.NewRaw().WithConnector("meter").WithValue([]byte{1}))
	if err != nil {
		t.Fatal(err)
	}
	if n := len(mapped.Updates[0].Values); n != 1 || mapped.Updates[0].Values[0].Path != "sometimes" {
		t.Errorf("got %+v, want only the path that produced a value", mapped.Updates[0].Values)
	}
}
