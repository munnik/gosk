package mapper

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUuidV7AtEmbedsTheGivenTime(t *testing.T) {
	base := uuid.Must(uuid.NewV7())
	want := time.Date(2026, 3, 4, 5, 6, 7, 891_000_000, time.UTC)

	got := uuidV7At(want, base)

	if v := got.Version(); v != 7 {
		t.Errorf("version is %d, want 7", v)
	}
	if v := got.Variant(); v != uuid.RFC4122 {
		t.Errorf("variant is %v, want RFC4122", v)
	}

	// The first 48 bits are the millisecond timestamp, big endian.
	ms := int64(got[0])<<40 | int64(got[1])<<32 | int64(got[2])<<24 |
		int64(got[3])<<16 | int64(got[4])<<8 | int64(got[5])
	if ms != want.UnixMilli() {
		t.Errorf("embedded time is %d ms, want %d ms (%s)", ms, want.UnixMilli(), want)
	}
}

func TestUuidV7AtIsDeterministic(t *testing.T) {
	// The same raw message and the same payload timestamp have to produce
	// the same UUID: re-processing must not silently change the identity
	// of a row the transfer protocol counts by UUID.
	base := uuid.Must(uuid.NewV7())
	at := time.Date(2026, 3, 4, 5, 6, 7, 891_000_000, time.UTC)

	if a, b := uuidV7At(at, base), uuidV7At(at, base); a != b {
		t.Errorf("got %s then %s for the same input", a, b)
	}
}

func TestUuidV7AtKeepsTheBaseEntropy(t *testing.T) {
	// Only the timestamp, version and variant are written; the rest
	// carries the raw message's own randomness, so two different raw
	// messages at the same instant stay distinct.
	at := time.Date(2026, 3, 4, 5, 6, 7, 891_000_000, time.UTC)
	first := uuidV7At(at, uuid.Must(uuid.NewV7()))
	second := uuidV7At(at, uuid.Must(uuid.NewV7()))

	if first == second {
		t.Errorf("two different base UUIDs both produced %s", first)
	}

	base := uuid.Must(uuid.NewV7())
	got := uuidV7At(at, base)
	for i := 9; i < 16; i++ {
		if got[i] != base[i] {
			t.Errorf("byte %d is %#x, want the base's %#x", i, got[i], base[i])
		}
	}
}

func TestUuidV7AtRoundTripsThroughTheSameArithmeticPostgresUses(t *testing.T) {
	// Mirrors what uuid_timestamp() does server side: milliseconds only,
	// so anything finer is expected to be lost.
	base := uuid.Must(uuid.NewV7())
	for _, at := range []time.Time{
		time.Unix(0, 0).UTC(),
		time.Date(2026, 9, 24, 16, 30, 20, 455_108_166, time.UTC),
		time.Date(2038, 1, 19, 3, 14, 8, 0, time.UTC),
	} {
		got := uuidV7At(at, base)
		ms := int64(got[0])<<40 | int64(got[1])<<32 | int64(got[2])<<24 |
			int64(got[3])<<16 | int64(got[4])<<8 | int64(got[5])
		if want := at.UnixMilli(); ms != want {
			t.Errorf("uuidV7At(%s) embedded %d ms, want %d", at, ms, want)
		}
	}
}
