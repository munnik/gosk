package message

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func float64Ptr(f float64) *float64 { return &f }
func stringPtr(s string) *string    { return &s }

// TestMappedMsgpRoundTripPreservesPolymorphicValues is the critical
// correctness test for the msgp integration: Value.Value is interface{},
// and mappedMsgp works around that by tagging each Value with its
// concrete type before encoding it natively (see value_msgp.go and
// mapped_msgp.go's doc comments) rather than guessing on the way back in
// - this specifically exercises the tricky cases (a struct-shaped
// Position, a Notification with a nil field, a plain scalar), not just
// scalars, since those are exactly the values Decode's mapstructure-based
// fallback has to get right for anything NOT covered by a tag.
func TestMappedMsgpRoundTripPreservesPolymorphicValues(t *testing.T) {
	original := *NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:123456789").WithOrigin("vessels.urn:mrn:imo:mmsi:123456789").
		AddUpdate(
			NewUpdate().WithSource(
				*NewSource().WithLabel("GPS").WithType("nmea0183").WithUuid(uuid.MustParse("11111111-2222-3333-4444-555555555555")).WithTransferUuid(uuid.MustParse("66666666-7777-8888-9999-aaaaaaaaaaaa")),
			).WithTimestamp(time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)).
				AddValue(NewValue().WithPath("navigation.position").WithValue(Position{Latitude: float64Ptr(52.1), Longitude: float64Ptr(5.9)})).
				AddValue(NewValue().WithPath("notifications.test.alarm").WithValue(Notification{State: stringPtr("alarm"), Method: []string{"sound", "visual"}, Message: stringPtr("test")})).
				AddValue(NewValue().WithPath("propulsion.mainEnginePort.drive.torque").WithValue(1234.5)),
		).
		AddUpdate(
			NewUpdate().WithSource(*NewSource().WithLabel("manner").WithType("binary")).WithTimestamp(time.Now()).
				AddValue(NewValue().WithPath("propulsion.mainEnginePort.drive.revolutions").WithValue(1500.0)).
				AddValue(NewValue().WithPath("notifications.test.cleared").WithValue(nil)),
		)

	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("MarshalMsg: %v", err)
	}

	var decoded Mapped
	if _, err := decoded.UnmarshalMsg(data); err != nil {
		t.Fatalf("UnmarshalMsg: %v", err)
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

	// specifically confirm the polymorphic types survived as their real
	// Go types, not degraded to a generic map - this is what Decode's
	// dispatch (invoked via Value.UnmarshalJSON on each embedded blob)
	// is responsible for.
	pos, ok := decoded.Updates[0].Values[0].Value.(Position)
	if !ok {
		t.Fatalf("expected Position, got %T: %+v", decoded.Updates[0].Values[0].Value, decoded.Updates[0].Values[0].Value)
	}
	if pos.Latitude == nil || *pos.Latitude != 52.1 {
		t.Fatalf("unexpected Position: %+v", pos)
	}

	notif, ok := decoded.Updates[0].Values[1].Value.(Notification)
	if !ok {
		t.Fatalf("expected Notification, got %T: %+v", decoded.Updates[0].Values[1].Value, decoded.Updates[0].Values[1].Value)
	}
	if notif.State == nil || *notif.State != "alarm" {
		t.Fatalf("unexpected Notification: %+v", notif)
	}

	if decoded.Updates[1].Values[1].Value != nil {
		t.Fatalf("expected a cleared (nil) notification value, got %+v", decoded.Updates[1].Values[1].Value)
	}
}

func TestRawMsgpRoundTrip(t *testing.T) {
	original := Raw{
		Connector: "testConnector",
		Timestamp: time.Now(),
		Type:      "manner_ethernet",
		Uuid:      uuid.New(),
		Value:     []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01},
	}

	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("MarshalMsg: %v", err)
	}

	var decoded Raw
	if _, err := decoded.UnmarshalMsg(data); err != nil {
		t.Fatalf("UnmarshalMsg: %v", err)
	}

	if !decoded.Equals(original) {
		t.Fatalf("round-tripped value differs:\n  got:  %+v\n  want: %+v", decoded, original)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Fatalf("round-tripped timestamp differs: got %v, want %v", decoded.Timestamp, original.Timestamp)
	}
	if decoded.Uuid != original.Uuid {
		t.Fatalf("round-tripped uuid differs: got %v, want %v", decoded.Uuid, original.Uuid)
	}
}

func TestRawMsgpRoundTripNilValue(t *testing.T) {
	original := Raw{Connector: "c", Timestamp: time.Now(), Type: "t", Uuid: uuid.New(), Value: nil}

	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("MarshalMsg: %v", err)
	}
	var decoded Raw
	if _, err := decoded.UnmarshalMsg(data); err != nil {
		t.Fatalf("UnmarshalMsg: %v", err)
	}
	if len(decoded.Value) != 0 {
		t.Fatalf("expected empty Value, got %+v", decoded.Value)
	}
}

func TestMappedMarshalMsgReusesBuffer(t *testing.T) {
	m := sampleMapped()
	buf := make([]byte, 0, 4)
	out, err := m.MarshalMsg(buf)
	if err != nil {
		t.Fatalf("MarshalMsg: %v", err)
	}

	var decoded Mapped
	if _, err := decoded.UnmarshalMsg(out); err != nil {
		t.Fatalf("UnmarshalMsg: %v", err)
	}
	if !decoded.Equals(m) {
		t.Fatalf("round-tripped value differs after buffer reuse:\n  got:  %+v\n  want: %+v", decoded, m)
	}
}

func BenchmarkMappedMsgpMarshal(b *testing.B) {
	m := sampleMapped()
	buf := make([]byte, 0, 512)
	b.ResetTimer()
	for b.Loop() {
		var err error
		buf, err = m.MarshalMsg(buf[:0])
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMappedMsgpUnmarshal(b *testing.B) {
	m := sampleMapped()
	data, err := m.MarshalMsg(nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var decoded Mapped
		if _, err := decoded.UnmarshalMsg(data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawMsgpMarshal(b *testing.B) {
	r := Raw{
		Connector: "testConnector",
		Timestamp: time.Now(),
		Type:      "manner_ethernet",
		Uuid:      uuid.New(),
		Value:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c},
	}
	buf := make([]byte, 0, 128)
	b.ResetTimer()
	for b.Loop() {
		var err error
		buf, err = r.MarshalMsg(buf[:0])
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRawMsgpUnmarshal(b *testing.B) {
	r := Raw{
		Connector: "testConnector",
		Timestamp: time.Now(),
		Type:      "manner_ethernet",
		Uuid:      uuid.New(),
		Value:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c},
	}
	data, err := r.MarshalMsg(nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var decoded Raw
		if _, err := decoded.UnmarshalMsg(data); err != nil {
			b.Fatal(err)
		}
	}
}
