package transfer

import (
	"fmt"
	"strings"
	"time"

	"github.com/munnik/gosk/database"
)

// What the far end checks before it believes a message.
//
// The broker is what knows who sent a message: its ACL lets a vessel write
// only to the topics that end in its own origin (sumo-mqtt.nix in the nix
// repo). Everything inside the payload is the sender's own say-so. The
// receiver used to store a delivery under the origin its envelope named, and
// each row under the origin its delta named, so a vessel allowed to publish
// on outbox/data/A could write rows into B's history - and, worse, record a
// high generation for one of B's buckets, after which every genuine delivery
// of that bucket from B is turned away as stale. The topic is therefore the
// identity, and the payload has to agree with it throughout.

// originFromTopic returns the origin a topic made from pattern names, and
// false when topic is not one of pattern's.
func originFromTopic(pattern, topic string) (string, bool) {
	prefix, suffix, ok := strings.Cut(pattern, "%s")
	if !ok || !strings.HasPrefix(topic, prefix) || !strings.HasSuffix(topic, suffix) {
		return "", false
	}
	origin := strings.TrimSuffix(strings.TrimPrefix(topic, prefix), suffix)
	if origin == "" || strings.ContainsAny(origin, "/+#") {
		return "", false
	}
	return origin, true
}

// validateDelivery checks that a delivery published on topic is what a
// sender (send.go, database.ReadOutboxDelivery) can produce for the origin
// the topic names:
//
//   - the envelope and every delta name that origin;
//   - the bucket is one whole OutboxBucket and the generation one the
//     sequence can have given out (it starts at 1);
//   - through lies within the bucket, and no row is newer than it or
//     outside the bucket.
//
// An empty delivery is valid: a bucket whose latest ran ahead of its rows is
// settled with one (the empty seeded tail, see ReadOutboxDelivery), and its
// through is then the bucket's latest, which is inside the bucket too.
func validateDelivery(topic string, d *Delivery) error {
	origin, ok := originFromTopic(dataTopic, topic)
	if !ok {
		return fmt.Errorf("topic %q is not an outbox data topic", topic)
	}
	if d.Origin != origin {
		return fmt.Errorf("delivery names origin %q but was published on %q", d.Origin, topic)
	}
	if err := validateBucket(d.Bucket); err != nil {
		return err
	}
	if d.Generation <= 0 {
		return fmt.Errorf("generation %d is not one the outbox sequence gives out", d.Generation)
	}
	end := d.Bucket.Add(database.OutboxBucket)
	if d.Through.Before(d.Bucket) || !d.Through.Before(end) {
		return fmt.Errorf("through %v is outside bucket %v", d.Through, d.Bucket)
	}
	for i := range d.Deltas {
		delta := &d.Deltas[i]
		if delta.Origin != origin {
			return fmt.Errorf("delta %d names origin %q in a delivery from %q", i, delta.Origin, origin)
		}
		for _, u := range delta.Updates {
			if u.Timestamp.Before(d.Bucket) || u.Timestamp.After(d.Through) {
				return fmt.Errorf("delta %d has a row at %v, outside %v..%v", i, u.Timestamp, d.Bucket, d.Through)
			}
		}
	}
	return nil
}

func validateBucket(bucket time.Time) error {
	if bucket.IsZero() || !bucket.Truncate(database.OutboxBucket).Equal(bucket) {
		return fmt.Errorf("bucket %v is not the start of an outbox bucket", bucket)
	}
	return nil
}

// validateOffer checks that a digest offer published on topic is about the
// origin the topic names. An offer about someone else gets nothing from the
// reply - it goes to that vessel's reply topic, and that vessel discards a
// reply to an exchange it did not start - but it would still make the far
// end run a long read over another vessel's history on request.
func validateOffer(topic string, o *DigestOffer) error {
	origin, ok := originFromTopic(digestTopic, topic)
	if !ok {
		return fmt.Errorf("topic %q is not an outbox digest topic", topic)
	}
	if o.Origin != origin {
		return fmt.Errorf("offer names origin %q but was published on %q", o.Origin, topic)
	}
	return nil
}
