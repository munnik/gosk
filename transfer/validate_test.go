package transfer

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/message"
)

const (
	vesselA = "vessels.urn:mrn:imo:mmsi:244000001"
	vesselB = "vessels.urn:mrn:imo:mmsi:244000002"
)

var validationBucket = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// genuineDelivery is what send.go publishes for vessel A: its own rows and
// an AIS row about B, which is legitimate - context is what the data is
// about, origin is who wrote it.
func genuineDelivery() Delivery {
	update := func(at time.Time) message.Update {
		return message.Update{Source: message.Source{Label: "gps", Type: "nmea0183"}, Timestamp: at, Values: []message.Value{{Path: "navigation.speedOverGround", Value: 1.0}}}
	}
	return Delivery{
		Delivery:   uuid.New(),
		Origin:     vesselA,
		Bucket:     validationBucket,
		Generation: 42,
		Through:    validationBucket.Add(30 * time.Second),
		Deltas: []message.Mapped{
			{Context: vesselA, Origin: vesselA, Updates: []message.Update{update(validationBucket.Add(10 * time.Second))}},
			{Context: vesselB, Origin: vesselA, Updates: []message.Update{update(validationBucket.Add(30 * time.Second))}},
		},
	}
}

func TestValidateDeliveryAcceptsWhatASenderSends(t *testing.T) {
	d := genuineDelivery()
	if err := validateDelivery("outbox/data/"+vesselA, &d); err != nil {
		t.Fatalf("genuine delivery rejected: %v", err)
	}
}

func TestValidateDeliveryAcceptsAnEmptySeededTail(t *testing.T) {
	// ReadOutboxDelivery found nothing newer than acked_through and sent
	// through = latest with no rows, to settle the bucket.
	d := genuineDelivery()
	d.Deltas = nil
	d.Through = validationBucket.Add(59 * time.Second)
	if err := validateDelivery("outbox/data/"+vesselA, &d); err != nil {
		t.Fatalf("empty seeded tail rejected: %v", err)
	}
}

func TestValidateDeliveryRejectsAForgedEnvelope(t *testing.T) {
	// Vessel A may publish only on its own topic, and names B inside.
	d := genuineDelivery()
	d.Origin = vesselB
	if err := validateDelivery("outbox/data/"+vesselA, &d); err == nil {
		t.Fatal("a delivery naming B on A's topic was accepted")
	}
}

func TestValidateDeliveryRejectsAForgedNestedOrigin(t *testing.T) {
	// The envelope is A's, but one delta claims to be B's rows.
	d := genuineDelivery()
	d.Deltas[1].Origin = vesselB
	if err := validateDelivery("outbox/data/"+vesselA, &d); err == nil {
		t.Fatal("a delta with B's origin in A's delivery was accepted")
	}
}

func TestValidateDeliveryRejectsWhatNoSenderProduces(t *testing.T) {
	for name, mutate := range map[string]func(*Delivery){
		"bucket not on a minute": func(d *Delivery) { d.Bucket = d.Bucket.Add(time.Second) },
		"zero bucket":            func(d *Delivery) { d.Bucket = time.Time{} },
		"generation zero":        func(d *Delivery) { d.Generation = 0 },
		"through past the bucket": func(d *Delivery) {
			d.Through = validationBucket.Add(time.Minute)
		},
		"through before the bucket": func(d *Delivery) {
			d.Through = validationBucket.Add(-time.Second)
		},
		"row newer than through": func(d *Delivery) {
			d.Through = validationBucket.Add(20 * time.Second)
		},
		"row before the bucket": func(d *Delivery) {
			d.Deltas[0].Updates[0].Timestamp = validationBucket.Add(-time.Second)
		},
	} {
		d := genuineDelivery()
		mutate(&d)
		if err := validateDelivery("outbox/data/"+vesselA, &d); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestValidateDeliveryRejectsOtherTopics(t *testing.T) {
	d := genuineDelivery()
	for _, topic := range []string{"outbox/ack/" + vesselA, "outbox/data/", "outbox/data/" + vesselA + "/x", "vessels/" + vesselA} {
		if err := validateDelivery(topic, &d); err == nil {
			t.Errorf("topic %q accepted", topic)
		}
	}
}

func TestValidateOfferBindsTheOriginToTheTopic(t *testing.T) {
	offer := DigestOffer{Exchange: uuid.New(), Origin: vesselA, Width: time.Hour}
	if err := validateOffer("outbox/digest/"+vesselA, &offer); err != nil {
		t.Fatalf("genuine offer rejected: %v", err)
	}
	offer.Origin = vesselB
	if err := validateOffer("outbox/digest/"+vesselA, &offer); err == nil {
		t.Fatal("an offer about B on A's topic was accepted")
	}
}

func TestCodecRefusesAPayloadThatDecompressesPastTheLimit(t *testing.T) {
	encoder, _ := zstd.NewWriter(nil)
	// Valid JSON, so that only the size limit can be what refuses it.
	plain := append(append([]byte(`{"origin":"`), bytes.Repeat([]byte{'a'}, maxDecodedPayload)...), `"}`...)
	bomb := encoder.EncodeAll(plain, nil)
	if len(bomb) > 1<<20 {
		t.Fatalf("test payload is not small: %d bytes", len(bomb))
	}
	var d Delivery
	if err := newCodec(false).decode(bomb, &d); err == nil {
		t.Fatal("a payload decompressing past the limit was decoded")
	}
	// And the same shape just under the limit still decodes.
	small := append(append([]byte(`{"origin":"`), bytes.Repeat([]byte{'a'}, 1<<20)...), `"}`...)
	if err := newCodec(false).decode(encoder.EncodeAll(small, nil), &d); err != nil {
		t.Fatalf("a payload under the limit was refused: %v", err)
	}
}

func TestEncodeSizedReportsTheDecodedSize(t *testing.T) {
	d := genuineDelivery()
	for _, compress := range []bool{true, false} {
		payload, plain, err := newCodec(compress).encodeSized(d)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Delivery
		if err := newCodec(false).decode(payload, &decoded); err != nil {
			t.Fatal(err)
		}
		reencoded, _ := json.Marshal(decoded)
		if plain != len(reencoded) {
			t.Errorf("compress=%v: encodeSized said %d bytes decoded, the payload decodes to %d", compress, plain, len(reencoded))
		}
	}
}

func TestAcknowledgementsOfAnEarlierRunAreNotApplied(t *testing.T) {
	for _, c := range []struct {
		ack, run int64
		want     bool
	}{
		{ack: 7, run: 7, want: true},
		{ack: 6, run: 7, want: false},
		{ack: 8, run: 7, want: false},
		// From before this sender knew epochs, queued for its session.
		{ack: 0, run: 7, want: false},
		// Anything before the reservation is done.
		{ack: 0, run: 0, want: false},
		{ack: 3, run: 0, want: false},
	} {
		if got := ackOfRun(Ack{Epoch: c.ack}, c.run); got != c.want {
			t.Errorf("ackOfRun(epoch %d, run %d) = %v, want %v", c.ack, c.run, got, c.want)
		}
	}
}
