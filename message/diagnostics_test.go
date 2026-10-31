package message

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
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
	s := NewSource().WithLabel("connectAmpero").WithType("modbus")
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

func TestNULIsNotADiagnostic(t *testing.T) {
	if _, err := NormalizeDiagnostic("open\x00loop"); err == nil {
		t.Fatal("a string with a NUL normalised; jsonb would refuse it")
	}
	got := NormalizeDiagnostics(map[string]any{"ok": 1.0, "bad": "x\x00", "nul\x00key": true, "": 2.0})
	if len(got) != 1 || got["ok"] != 1.0 {
		t.Fatalf("got %+v, want only ok", got)
	}

	var v Value
	if err := json.Unmarshal([]byte(`{"path":"a.b","value":1,"diagnostics":{"s":"x\u0000","k\u0000":1,"ok":true}}`), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Diagnostics) != 1 || v.Diagnostics["ok"] != true {
		t.Fatalf("got %+v", v.Diagnostics)
	}
}

// A diagnostics field that does not decode - here a number beyond float64 -
// must cost the diagnostics, not the value and not the batch it travels in.
func TestUndecodableDiagnosticsKeepTheValue(t *testing.T) {
	var v Value
	if err := json.Unmarshal([]byte(`{"path":"a.b","value":2,"diagnostics":{"raw":1e400}}`), &v); err != nil {
		t.Fatalf("the value failed over its diagnostics: %v", err)
	}
	if v.Path != "a.b" || v.Value != 2.0 || v.Diagnostics != nil {
		t.Fatalf("got %+v", v)
	}

	batch := []byte(`{"context":"c","origin":"c","updates":[{"source":{"label":"l","type":"modbus","uuid":"00000000-0000-0000-0000-000000000000","transferUuid":"00000000-0000-0000-0000-000000000000"},"timestamp":"2026-10-17T06:00:00Z","values":[{"path":"a.b","value":1,"diagnostics":{"raw":1e400}},{"path":"c.d","value":2,"diagnostics":{"current":0.01}}]}]}`)
	var m Mapped
	if err := json.Unmarshal(batch, &m); err != nil {
		t.Fatalf("the batch failed over one value's diagnostics: %v", err)
	}
	if d := diagnosticsOf(&m); d["a.b"] != nil || d["c.d"]["current"] != 0.01 {
		t.Fatalf("got %+v", d)
	}

	var s SingleValueMapped
	if err := json.Unmarshal([]byte(`{"context":"c","origin":"c","source":{"label":"l","type":"t","uuid":"00000000-0000-0000-0000-000000000000","transferUuid":"00000000-0000-0000-0000-000000000000"},"timestamp":"2026-10-17T06:00:00Z","path":"a.b","value":3,"diagnostics":{"raw":1e400}}`), &s); err != nil || s.Value != 3.0 || s.Diagnostics != nil {
		t.Fatalf("SingleValueMapped: %+v, %v", s, err)
	}
	// and a value itself that does not decode still fails, as before
	if err := json.Unmarshal([]byte(`{"path":"a.b","value":1e400}`), &v); err == nil {
		t.Fatal("a value beyond float64 decoded")
	}
	if err := json.Unmarshal([]byte(`{"path":null,"value":1}`), &v); err == nil {
		t.Fatal("a null path decoded")
	}
}

// Details are the one object a diagnostics field holds: their leaves are
// normalised like the top level's, and anything deeper, or a details that
// is not an object, is dropped without costing the rest.
func TestNormalizeDiagnosticsKeepsDetailsOneLevelDown(t *testing.T) {
	got := NormalizeDiagnostics(map[string]any{
		"connected": true,
		DiagnosticDetails: map[string]any{
			"rawVoltage": 4.8,
			"images":     uint16(12),
			"nested":     map[string]any{"a": 1.0},
			"bad":        math.NaN(),
		},
	})
	details, ok := got[DiagnosticDetails].(map[string]any)
	if got["connected"] != true || !ok || len(details) != 2 || details["rawVoltage"] != 4.8 || details["images"] != 12.0 {
		t.Fatalf("got %#v", got)
	}

	for _, in := range []map[string]any{
		{"trust": 0.5, DiagnosticDetails: "not an object"},
		{"trust": 0.5, DiagnosticDetails: map[string]any{}},
		{"trust": 0.5, DiagnosticDetails: map[string]any{"nested": map[string]any{"a": 1.0}}},
	} {
		if got := NormalizeDiagnostics(in); len(got) != 1 || got["trust"] != 0.5 {
			t.Errorf("NormalizeDiagnostics(%#v) = %#v, want only the trust", in, got)
		}
	}
	if got := NormalizeDiagnostics(map[string]any{"connected": map[string]any{"a": 1.0}}); got != nil {
		t.Errorf("an object under a key other than details survived: %#v", got)
	}
}

func TestDetailsSurviveJSONAndMsgpack(t *testing.T) {
	d := map[string]any{"connected": true, DiagnosticDetails: map[string]any{"current": 0.012, "inputConnected": true}}
	in := NewMapped().WithContext("c").WithOrigin("c").AddUpdate(
		NewUpdate().WithSource(*NewSource().WithLabel("l").WithType("modbus")).
			WithTimestamp(time.Date(2026, 10, 17, 6, 0, 0, 0, time.UTC)).
			AddValue(NewValue().WithPath("tanks.fuel.aft.currentLevel").WithValue(0.5).WithDiagnostics(d)))

	blob, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var fromJSON Mapped
	if err := json.Unmarshal(blob, &fromJSON); err != nil {
		t.Fatal(err)
	}
	pack, err := in.MarshalMsg(nil)
	if err != nil {
		t.Fatal(err)
	}
	var fromMsgpack Mapped
	if _, err := fromMsgpack.UnmarshalMsg(pack); err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]Mapped{"JSON": fromJSON, "msgpack": fromMsgpack} {
		if !in.Equals(out) {
			t.Errorf("%s round trip changed the message:\n in: %+v\nout: %+v", name, *in, out)
		}
	}
}
