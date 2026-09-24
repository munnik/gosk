package message

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/munnik/uuid/v5"
)

// These mirror Mapped/Update/Source's own field layout exactly, but
// without the easyjson-generated MarshalJSON/UnmarshalJSON methods, so
// json.Marshal falls back to its default reflection-based struct encoder -
// the same code path these types used before easyjson generated fast
// paths for them. Comparing output against these is the strongest
// evidence the codegen changed nothing observable about the wire format.
type plainSource struct {
	Label        string    `json:"label"`
	Type         string    `json:"type"`
	Uuid         uuid.UUID `json:"uuid"`
	TransferUuid uuid.UUID `json:"transferUuid"`
}

type plainUpdate struct {
	Source    plainSource `json:"source"`
	Timestamp time.Time   `json:"timestamp"`
	Values    []Value     `json:"values"`
}

type plainMapped struct {
	Context string        `json:"context"`
	Origin  string        `json:"origin"`
	Updates []plainUpdate `json:"updates"`
}

func toPlainSource(s Source) plainSource {
	return plainSource{Label: s.Label, Type: s.Type, Uuid: s.Uuid, TransferUuid: s.TransferUuid}
}

func toPlainUpdate(u Update) plainUpdate {
	return plainUpdate{Source: toPlainSource(u.Source), Timestamp: u.Timestamp, Values: u.Values}
}

func toPlainMapped(m Mapped) plainMapped {
	updates := make([]plainUpdate, len(m.Updates))
	for i, u := range m.Updates {
		updates[i] = toPlainUpdate(u)
	}
	return plainMapped{Context: m.Context, Origin: m.Origin, Updates: updates}
}

func sampleMapped() Mapped {
	return *NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:123456789").WithOrigin("vessels.urn:mrn:imo:mmsi:123456789").
		AddUpdate(
			NewUpdate().WithSource(
				*NewSource().WithLabel("GPS").WithType("nmea0183").WithUuid(uuid.Must(uuid.FromString("11111111-2222-3333-4444-555555555555"))).WithTransferUuid(uuid.Must(uuid.FromString("66666666-7777-8888-9999-aaaaaaaaaaaa"))),
			).WithTimestamp(time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)).
				// plain scalars only: Value.UnmarshalJSON's Decode dispatch
				// (unrelated to this test) recognizes some map shapes -
				// e.g. {latitude, longitude} - and converts them to typed
				// structs like Position on decode, which is correct,
				// pre-existing behavior this test isn't about; using
				// scalars here keeps the round trip an apples-to-apples
				// comparison instead of tripping over that.
				AddValue(NewValue().WithPath("navigation.speedOverGround").WithValue(5.4)).
				AddValue(NewValue().WithPath("propulsion.mainEnginePort.drive.torque").WithValue(1234.5)),
		).
		AddUpdate(
			NewUpdate().WithSource(*NewSource().WithLabel("manner").WithType("binary")).WithTimestamp(time.Now()).
				// float64, not a bare int literal: any JSON number decoded
				// into interface{} comes back as float64 (encoding/json's
				// universal behavior), so an int here would make Equals
				// correctly report a type mismatch after the round trip -
				// a test artifact, not something this round trip is
				// actually about.
				AddValue(NewValue().WithPath("propulsion.mainEnginePort.drive.revolutions").WithValue(1500.0)),
		)
}

func TestMappedMarshalJSONMatchesDefaultReflectionEncoding(t *testing.T) {
	m := sampleMapped()

	want, err := json.Marshal(toPlainMapped(m))
	if err != nil {
		t.Fatalf("json.Marshal(plainMapped): %v", err)
	}
	got, err := m.MarshalJSON()
	if err != nil {
		t.Fatalf("Mapped.MarshalJSON: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("Mapped.MarshalJSON output changed:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestMappedMarshalUnmarshalRoundTrip(t *testing.T) {
	original := sampleMapped()

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var decoded Mapped
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if !decoded.Equals(original) {
		t.Fatalf("round-tripped value differs:\n  got:  %+v\n  want: %+v", decoded, original)
	}
	for i := range original.Updates {
		if decoded.Updates[i].Source != original.Updates[i].Source {
			t.Fatalf("round-tripped source[%d] differs: got %+v, want %+v", i, decoded.Updates[i].Source, original.Updates[i].Source)
		}
		if !decoded.Updates[i].Timestamp.Equal(original.Updates[i].Timestamp) {
			t.Fatalf("round-tripped timestamp[%d] differs: got %v, want %v", i, decoded.Updates[i].Timestamp, original.Updates[i].Timestamp)
		}
	}
}

func TestMappedUnmarshalJSONFromStandardEncoderOutput(t *testing.T) {
	// Something that marshaled Mapped BEFORE this process's easyjson
	// upgrade (an older gosk build on another vessel node, say) still
	// needs to be readable - UnmarshalJSON must accept the plain
	// reflection encoder's output just as well as its own.
	m := sampleMapped()
	data, err := json.Marshal(toPlainMapped(m))
	if err != nil {
		t.Fatalf("json.Marshal(plainMapped): %v", err)
	}

	var decoded Mapped
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !decoded.Equals(m) {
		t.Fatalf("decoded value differs:\n  got:  %+v\n  want: %+v", decoded, m)
	}
}

func BenchmarkMappedMarshalJSON(b *testing.B) {
	m := sampleMapped()
	b.ResetTimer()
	for b.Loop() {
		if _, err := m.MarshalJSON(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMappedMarshalJSONPlain(b *testing.B) {
	m := toPlainMapped(sampleMapped())
	b.ResetTimer()
	for b.Loop() {
		if _, err := json.Marshal(m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMappedUnmarshalJSON(b *testing.B) {
	m := sampleMapped()
	data, err := m.MarshalJSON()
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var decoded Mapped
		if err := decoded.UnmarshalJSON(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMappedUnmarshalJSONPlain(b *testing.B) {
	m := toPlainMapped(sampleMapped())
	data, err := json.Marshal(m)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var decoded plainMapped
		if err := json.Unmarshal(data, &decoded); err != nil {
			b.Fatal(err)
		}
	}
}
