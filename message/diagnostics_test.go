package message

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNormalizeDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want any
	}{
		{true, true},
		{"all zeros", "all zeros"},
		{0.012, 0.012},
		{uint16(12000), 12000.0},
		{int(-3), -3.0},
		{int64(7), 7.0},
		{float32(0.5), 0.5},
	} {
		got, err := NormalizeDiagnostic(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("NormalizeDiagnostic(%#v) = %#v, %v; want %#v", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []any{nil, math.NaN(), math.Inf(1), []int{1}, map[string]any{"a": 1.0}, struct{}{}} {
		if got, err := NormalizeDiagnostic(in); err == nil {
			t.Errorf("NormalizeDiagnostic(%#v) = %#v, want an error", in, got)
		}
	}
}

func mappedWithDiagnostics() *Mapped {
	s := NewSource().WithLabel("connectAmpero").WithType("modbus").WithUuid(uuid.New())
	u := NewUpdate().WithSource(*s).WithTimestamp(time.Date(2026, 10, 17, 6, 0, 0, 0, time.UTC))
	u.AddValue(NewValue().WithPath("tanks.fuel.aft.currentLevel").WithValue(0.5).WithDiagnostics(map[string]any{"current": 0.012, "connected": true}))
	u.AddValue(NewValue().WithPath("tanks.fuel.forward.currentLevel").WithValue(nil).WithDiagnostics(map[string]any{DiagnosticRejected: "all zeros", "current": 0.0}))
	u.AddValue(NewValue().WithPath("navigation.speedOverGround").WithValue(3.2))
	return NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:244000000").WithOrigin("vessels.urn:mrn:imo:mmsi:244000000").AddUpdate(u)
}

func diagnosticsOf(m *Mapped) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for _, svm := range m.ToSingleValueMapped() {
		result[svm.Path] = svm.Diagnostics
	}
	return result
}

func TestDiagnosticsSurviveJSON(t *testing.T) {
	in := mappedWithDiagnostics()
	blob, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(blob), `"diagnostics"`); got != 2 {
		t.Fatalf("want diagnostics written for the two values that have them, found %d in %s", got, blob)
	}
	var out Mapped
	if err := json.Unmarshal(blob, &out); err != nil {
		t.Fatal(err)
	}
	if !in.Equals(out) {
		t.Fatalf("round trip changed the message:\n in: %+v\nout: %+v", *in, out)
	}
	d := diagnosticsOf(&out)
	if d["tanks.fuel.aft.currentLevel"]["current"] != 0.012 || d["tanks.fuel.aft.currentLevel"]["connected"] != true {
		t.Fatalf("aft diagnostics lost: %+v", d)
	}
	if d["tanks.fuel.forward.currentLevel"][DiagnosticRejected] != "all zeros" {
		t.Fatalf("rejection lost: %+v", d)
	}
	if d["navigation.speedOverGround"] != nil {
		t.Fatalf("a value without diagnostics gained some: %+v", d)
	}
}

func TestJSONFromAGoskWithoutDiagnostics(t *testing.T) {
	blob := []byte(`{"context":"c","origin":"c","updates":[{"source":{"label":"l","type":"modbus","uuid":"00000000-0000-0000-0000-000000000000","transferUuid":"00000000-0000-0000-0000-000000000000"},"timestamp":"2026-10-17T06:00:00Z","values":[{"path":"a.b","value":1.5}]}]}`)
	var m Mapped
	if err := json.Unmarshal(blob, &m); err != nil {
		t.Fatal(err)
	}
	if v := m.Updates[0].Values[0]; v.Value != 1.5 || v.Diagnostics != nil {
		t.Fatalf("got %+v", v)
	}
}

func TestDiagnosticsThatAreNotAnObjectAreIgnored(t *testing.T) {
	var v Value
	if err := json.Unmarshal([]byte(`{"path":"a.b","value":2,"diagnostics":[1,2]}`), &v); err != nil {
		t.Fatalf("a malformed diagnostics field failed the value: %v", err)
	}
	if v.Value != 2.0 || v.Diagnostics != nil {
		t.Fatalf("got %+v", v)
	}
}

func TestDiagnosticsSurviveMsgpack(t *testing.T) {
	in := mappedWithDiagnostics()
	blob, err := in.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}
	var out Mapped
	if _, err := out.UnmarshalMsg(blob); err != nil {
		t.Fatal(err)
	}
	if !in.Equals(out) {
		t.Fatalf("round trip changed the message:\n in: %+v\nout: %+v", *in, out)
	}
	if d := diagnosticsOf(&out); d["tanks.fuel.forward.currentLevel"][DiagnosticRejected] != "all zeros" || d["navigation.speedOverGround"] != nil {
		t.Fatalf("got %+v", d)
	}
}

// A gosk that predates diagnostics reads only a blob's payload and drops
// the rest unread: what it gets must be the value as it was.
func TestValueBlobReadByAGoskWithoutDiagnostics(t *testing.T) {
	v := *NewValue().WithPath("tanks.fuel.aft.currentLevel").WithValue(0.5).WithDiagnostics(map[string]any{"current": 0.012})
	blob, err := marshalValueMsg(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	old, tail, err := unmarshalValuePayload(blob)
	if err != nil {
		t.Fatal(err)
	}
	if old.Path != v.Path || old.Value != v.Value || len(tail) == 0 {
		t.Fatalf("an older reader would see %+v with a %d byte tail", old, len(tail))
	}

	plain, err := marshalValueMsg(*NewValue().WithPath("a").WithValue(1.0), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, tail, err := unmarshalValuePayload(plain); err != nil || len(tail) != 0 {
		t.Fatalf("a value without diagnostics left a %d byte tail (%v)", len(tail), err)
	}
}

func TestSingleValueMappedCarriesDiagnostics(t *testing.T) {
	svm := mappedWithDiagnostics().ToSingleValueMapped()[1]
	if svm.Diagnostics[DiagnosticRejected] != "all zeros" {
		t.Fatalf("ToSingleValueMapped lost them: %+v", svm)
	}
	if back := svm.ToMapped().ToSingleValueMapped()[0]; !back.Equals(svm) || back.Diagnostics["current"] != 0.0 {
		t.Fatalf("ToMapped lost them: %+v", back)
	}

	blob, err := svm.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}
	var fromMsgp SingleValueMapped
	if _, err := fromMsgp.UnmarshalMsg(blob); err != nil || !fromMsgp.Equals(svm) {
		t.Fatalf("msgpack: %+v, %v", fromMsgp, err)
	}

	js, err := json.Marshal(svm)
	if err != nil {
		t.Fatal(err)
	}
	var fromJSON SingleValueMapped
	if err := json.Unmarshal(js, &fromJSON); err != nil || !fromJSON.Equals(svm) {
		t.Fatalf("json: %+v, %v (%s)", fromJSON, err, js)
	}

	older := svm
	older.Diagnostics = map[string]any{"current": 1.0}
	if merged := older.Merge(svm); merged.Diagnostics[DiagnosticRejected] != "all zeros" {
		t.Fatalf("Merge kept the older reading's diagnostics: %+v", merged)
	}
}

func TestEqualsComparesDiagnostics(t *testing.T) {
	a := *NewValue().WithPath("a").WithValue(1.0)
	b := a
	b.Diagnostics = map[string]any{}
	if !a.Equals(b) {
		t.Fatal("nil and empty diagnostics should be equal")
	}
	b.Diagnostics = map[string]any{"current": 0.0}
	if a.Equals(b) {
		t.Fatal("values with different diagnostics compared equal")
	}
}
