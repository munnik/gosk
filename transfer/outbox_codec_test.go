package transfer

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
)

// shipment builds something the size of a real one: a batch of source
// messages, each mapping to a handful of paths.
func shipment(sources, pathsEach int) OutboxMessage {
	m := OutboxMessage{
		Shipment: uuid.Must(uuid.NewV7Precise()),
		Origin:   "vessels.urn:mrn:imo:mmsi:244770688",
		Uuids:    make([]uuid.UUID, 0, sources),
		Deltas:   make([]message.Mapped, 0, sources),
	}
	paths := []string{
		"propulsion.mainEngine.drive.torque",
		"propulsion.mainEngine.drive.revolutions",
		"propulsion.mainEngine.drive.power",
		"navigation.speedOverGround",
		"navigation.courseOverGroundTrue",
	}
	for i := range sources {
		at := time.Now().Add(time.Duration(i) * time.Millisecond).Truncate(time.Microsecond)
		u := uuid.Must(uuid.NewV7AtTimePrecise(at))
		m.Uuids = append(m.Uuids, u)

		s := message.NewSource().WithLabel("shaftPowerMeter").WithType("json").WithUuid(u)
		update := message.NewUpdate().WithSource(*s).WithTimestamp(at)
		for j := range pathsEach {
			update.AddValue(message.NewValue().WithPath(paths[j%len(paths)]).WithValue(float64(i*j) + 0.5))
		}
		m.Deltas = append(m.Deltas, *message.NewMapped().
			WithOrigin(m.Origin).WithContext(m.Origin).AddUpdate(update))
	}
	return m
}

func TestOutboxCodecRoundTrips(t *testing.T) {
	for _, compress := range []bool{true, false} {
		c := newOutboxCodec(compress)
		original := shipment(20, 3)

		payload, err := c.encode(original)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}

		var decoded OutboxMessage
		if err := c.decode(payload, &decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if decoded.Shipment != original.Shipment || decoded.Origin != original.Origin {
			t.Errorf("compress=%t: header did not survive", compress)
		}
		if len(decoded.Uuids) != len(original.Uuids) {
			t.Fatalf("compress=%t: %d uuids, want %d", compress, len(decoded.Uuids), len(original.Uuids))
		}
		for i := range original.Uuids {
			if decoded.Uuids[i] != original.Uuids[i] {
				t.Errorf("compress=%t: uuid %d differs", compress, i)
			}
		}
		if len(decoded.Deltas) != len(original.Deltas) {
			t.Errorf("compress=%t: %d deltas, want %d", compress, len(decoded.Deltas), len(original.Deltas))
		}
	}
}

// A compressed payload has to be recognisable as one, since that is what
// lets the two ends be switched over independently.
func TestOutboxCodecReadsEitherFormat(t *testing.T) {
	sender := newOutboxCodec(true)
	original := shipment(5, 2)

	compressed, err := sender.encode(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	plain, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// A receiver configured either way must read both, because whether a
	// payload is compressed is a property of the payload and not of the
	// receiver's configuration.
	for _, compress := range []bool{true, false} {
		receiver := newOutboxCodec(compress)
		for name, payload := range map[string][]byte{"compressed": compressed, "plain": plain} {
			var decoded OutboxMessage
			if err := receiver.decode(payload, &decoded); err != nil {
				t.Errorf("receiver compress=%t could not read a %s payload: %v", compress, name, err)
				continue
			}
			if decoded.Shipment != original.Shipment {
				t.Errorf("receiver compress=%t read a %s payload wrongly", compress, name)
			}
		}
	}
}

func TestOutboxCodecCompressionIsWorthIt(t *testing.T) {
	// A realistic batch: the shipper's default is 500 source messages.
	original := shipment(500, 3)

	plain, err := newOutboxCodec(false).encode(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	compressed, err := newOutboxCodec(true).encode(original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	ratio := float64(len(plain)) / float64(len(compressed))
	t.Logf("%d bytes plain, %d compressed, %.1fx", len(plain), len(compressed), ratio)

	// Deltas repeat their paths, their origin and their source labels, so
	// this should compress hard. The bound is deliberately loose - the
	// point is to catch compression silently not happening, not to pin a
	// ratio that depends on the zstd version.
	if ratio < 2 {
		t.Errorf("compressed to %.1fx, expected rather better than that on repetitive json", ratio)
	}
}

func TestOutboxCodecRejectsGarbage(t *testing.T) {
	c := newOutboxCodec(true)

	// Something that starts like a zstd frame but is not one: the error
	// has to come back rather than a half-decoded message.
	var decoded OutboxMessage
	if err := c.decode(append(append([]byte{}, zstdMagic...), 0xff, 0xff), &decoded); err == nil {
		t.Error("a truncated zstd frame decoded without error")
	}
	if err := c.decode([]byte("{not json"), &decoded); err == nil {
		t.Error("malformed json decoded without error")
	}
}
