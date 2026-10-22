package database

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/message"
)

func newTestBigCache(t *testing.T) *BigCache {
	t.Helper()
	return NewBigCache(&config.BigCacheConfig{LifeWindow: 60, HardMaxCacheSize: 16})
}

func TestBigCacheWriteReadRaw(t *testing.T) {
	c := newTestBigCache(t)
	raw := &message.Raw{
		Connector: "testConnector",
		Timestamp: time.Now(),
		Type:      "manner_ethernet",
		Uuid:      uuid.New(),
		Value:     []byte{0x01, 0x02, 0x03},
	}

	c.WriteRaw(raw, false)

	got, err := c.ReadRaw(raw.Uuid.String())
	if err != nil {
		t.Fatalf("ReadRaw: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if !got[0].Equals(*raw) {
		t.Fatalf("round-tripped raw differs:\n  got:  %+v\n  want: %+v", got[0], raw)
	}
	if !got[0].Timestamp.Equal(raw.Timestamp) {
		t.Fatalf("round-tripped timestamp differs: got %v, want %v", got[0].Timestamp, raw.Timestamp)
	}

	all, err := c.ReadRaw("")
	if err != nil {
		t.Fatalf("ReadRaw(\"\"): %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 result iterating all, got %d", len(all))
	}
}

func TestBigCacheWriteReadMapped(t *testing.T) {
	c := newTestBigCache(t)
	mapped := NewMappedWithPosition(t)

	c.WriteMapped(mapped)

	got, err := c.ReadMapped("")
	if err != nil {
		t.Fatalf("ReadMapped: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}

	sv := got[0].ToSingleValueMapped()
	if len(sv) != 1 {
		t.Fatalf("expected 1 single value, got %d", len(sv))
	}
	pos, ok := sv[0].Value.(message.Position)
	if !ok {
		t.Fatalf("expected Position, got %T: %+v", sv[0].Value, sv[0].Value)
	}
	if pos.Latitude == nil || *pos.Latitude != 52.1 {
		t.Fatalf("unexpected Position: %+v", pos)
	}
}

// NewMappedWithPosition builds a *message.Mapped with a single
// navigation.position Value, exercising the same polymorphic path
// value_msgp.go's tagged encoding exists for.
func NewMappedWithPosition(t *testing.T) *message.Mapped {
	t.Helper()
	lat := 52.1
	lon := 5.9
	return message.NewMapped().WithContext("vessels.urn:mrn:imo:mmsi:123456789").WithOrigin("vessels.urn:mrn:imo:mmsi:123456789").
		AddUpdate(
			message.NewUpdate().WithSource(*message.NewSource().WithLabel("GPS").WithType("nmea0183").WithUuid(uuid.New())).WithTimestamp(time.Now()).
				AddValue(message.NewValue().WithPath("navigation.position").WithValue(message.Position{Latitude: &lat, Longitude: &lon})),
		)
}
