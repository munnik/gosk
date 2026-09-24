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

// TestNewRawUuidAndTimestampAgree pins the single clock reading in NewRaw.
// The two fields describe the same instant, and once the UUID resolved to
// about 244ns rather than to a millisecond, reading the clock twice made
// them disagree by however long the second call took.
func TestNewRawUuidAndTimestampAgree(t *testing.T) {
	for range 1000 {
		r := NewRaw()

		// The 48 bit millisecond field, then the 12 bit sub-millisecond
		// fraction in 4096ths - the same arithmetic uuid_timestamp_micros
		// performs server side.
		ms := int64(r.Uuid[0])<<40 | int64(r.Uuid[1])<<32 | int64(r.Uuid[2])<<24 |
			int64(r.Uuid[3])<<16 | int64(r.Uuid[4])<<8 | int64(r.Uuid[5])
		frac := int64(r.Uuid[6]&0x0f)<<8 | int64(r.Uuid[7])
		embedded := time.UnixMilli(ms).Add(time.Duration(frac * int64(time.Millisecond) / 4096))

		// The timestamp must be storable as-is: timestamptz keeps
		// microseconds, and anything finer would be rounded on the way in,
		// leaving the stored row disagreeing with its own uuid.
		if r.Timestamp.Truncate(time.Microsecond) != r.Timestamp {
			t.Fatalf("timestamp %s is finer than a microsecond, so the database cannot store it unchanged",
				r.Timestamp.Format(time.RFC3339Nano))
		}

		// Within one step, which is all the format can express - not
		// "close enough", but as close as the encoding allows. A step is
		// 1000000/4096 = 244.14ns, which integer Duration arithmetic
		// truncates to 244, so the bound is the next nanosecond up. The
		// difference is never negative: the fraction is floored, so the
		// embedded time never runs ahead of the real one.
		const step = time.Millisecond/4096 + 1
		if d := r.Timestamp.Sub(embedded); d < 0 || d >= step {
			t.Fatalf("timestamp %s and uuid %s (embedding %s) differ by %s, want less than one %s step",
				r.Timestamp.Format(time.RFC3339Nano), r.Uuid, embedded.Format(time.RFC3339Nano), d, step)
		}
	}
}
