package message

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/munnik/uuid/v5"
)

// oldMarshalJSON is the map[string]string-based implementation this test
// guards against regressing away from: MarshalJSON must keep producing
// exactly what it produced, not just "valid JSON with the same values" -
// see rawWire's doc comment for why (a real consumer, or another gosk
// process's UnmarshalJSON, doesn't care about field order, but staying
// byte-identical is the strongest possible evidence the rewrite changed
// nothing observable).
func oldMarshalJSON(r Raw) ([]byte, error) {
	result := map[string]string{
		"connector": r.Connector,
		"timestamp": r.Timestamp.UTC().Format(time.RFC3339Nano),
		"type":      r.Type,
		"uuid":      r.Uuid.String(),
		"value":     base64.StdEncoding.EncodeToString(r.Value),
	}
	return json.Marshal(&result)
}

func TestRawMarshalJSONMatchesOldMapBasedImplementation(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	cases := []Raw{
		{
			Connector: "testConnector",
			Timestamp: time.Date(2026, 9, 17, 14, 30, 0, 123456789, loc),
			Type:      "modbus",
			Uuid:      uuid.Must(uuid.FromString("11111111-2222-3333-4444-555555555555")),
			Value:     []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01},
		},
		{
			// zero value: empty connector/type, nil Value, zero Uuid, zero time
			Connector: "",
			Timestamp: time.Time{},
			Type:      "",
			Uuid:      uuid.Nil,
			Value:     nil,
		},
		{
			Connector: "unicode-Ø-connector",
			Timestamp: time.Now(),
			Type:      "nmea0183",
			Uuid:      uuid.Must(uuid.NewV7Precise()),
			Value:     []byte("hello \"world\"\n"),
		},
	}

	for _, r := range cases {
		want, err := oldMarshalJSON(r)
		if err != nil {
			t.Fatalf("oldMarshalJSON: %v", err)
		}
		got, err := r.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("MarshalJSON output changed:\n  got:  %s\n  want: %s", got, want)
		}
	}
}

func TestRawMarshalUnmarshalRoundTrip(t *testing.T) {
	original := Raw{
		Connector: "testConnector",
		Timestamp: time.Now(),
		Type:      "modbus",
		Uuid:      uuid.Must(uuid.NewV7Precise()),
		Value:     []byte{1, 2, 3, 4, 5},
	}

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var decoded Raw
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}

	if !decoded.Equals(original) {
		t.Fatalf("round-tripped value differs: got %+v, want %+v", decoded, original)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Fatalf("round-tripped timestamp differs: got %v, want %v", decoded.Timestamp, original.Timestamp)
	}
	if decoded.Uuid != original.Uuid {
		t.Fatalf("round-tripped uuid differs: got %v, want %v", decoded.Uuid, original.Uuid)
	}
}

func BenchmarkRawMarshalJSON(b *testing.B) {
	r := Raw{
		Connector: "testConnector",
		Timestamp: time.Now(),
		Type:      "manner_ethernet",
		Uuid:      uuid.Must(uuid.NewV7Precise()),
		Value:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c},
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.MarshalJSON(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawMarshalJSONOld(b *testing.B) {
	r := Raw{
		Connector: "testConnector",
		Timestamp: time.Now(),
		Type:      "manner_ethernet",
		Uuid:      uuid.Must(uuid.NewV7Precise()),
		Value:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c},
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := oldMarshalJSON(r); err != nil {
			b.Fatal(err)
		}
	}
}
