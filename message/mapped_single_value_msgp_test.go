package message

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSingleValueMappedMsgpRoundTrip(t *testing.T) {
	original := SingleValueMapped{
		Context:   "vessels.urn:mrn:imo:mmsi:123456789",
		Origin:    "vessels.urn:mrn:imo:mmsi:123456789",
		Source:    *NewSource().WithLabel("GPS").WithType("nmea0183").WithUuid(uuid.MustParse("11111111-2222-3333-4444-555555555555")),
		Timestamp: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
		Path:      "navigation.position",
		Value:     Position{Latitude: float64Ptr(52.1), Longitude: float64Ptr(5.9)},
	}

	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("MarshalMsg: %v", err)
	}

	var decoded SingleValueMapped
	if _, err := decoded.UnmarshalMsg(data); err != nil {
		t.Fatalf("UnmarshalMsg: %v", err)
	}

	if !decoded.Equals(original) {
		t.Fatalf("round-tripped value differs:\n  got:  %+v\n  want: %+v", decoded, original)
	}
	if decoded.Source != original.Source {
		t.Fatalf("round-tripped source differs: got %+v, want %+v", decoded.Source, original.Source)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Fatalf("round-tripped timestamp differs: got %v, want %v", decoded.Timestamp, original.Timestamp)
	}
	pos, ok := decoded.Value.(Position)
	if !ok {
		t.Fatalf("expected Position, got %T: %+v", decoded.Value, decoded.Value)
	}
	if pos.Latitude == nil || *pos.Latitude != 52.1 {
		t.Fatalf("unexpected Position: %+v", pos)
	}
}
