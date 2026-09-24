package mapper

import (
	"time"

	"github.com/google/uuid"
)

// uuidV7At returns a version 7 UUID whose embedded timestamp is t, keeping
// the non-timestamp bits of base.
//
// It exists so that a row whose timestamp did not come from the raw
// message's arrival - see the timestampExpression handling in json.go -
// still carries a UUID that agrees with that timestamp. Without it, the
// UUID says when the bytes arrived and the row says when the measurement
// is for, and the two drift apart by however far the payload's clock is
// from ours.
//
// Reusing base's remaining bits rather than drawing fresh randomness keeps
// this deterministic: the same raw message mapped twice with the same
// payload timestamp produces the same UUID, so re-processing cannot
// silently change the identity of a row that the transfer protocol counts
// by UUID. base is the raw message's own version 7 UUID, so those bits are
// already random.
//
// Only the first 48 bits (the millisecond timestamp), the version nibble
// and the variant bits are written; everything else is base's.
func uuidV7At(t time.Time, base uuid.UUID) uuid.UUID {
	u := base

	ms := t.UnixMilli()
	u[0] = byte(ms >> 40)
	u[1] = byte(ms >> 32)
	u[2] = byte(ms >> 24)
	u[3] = byte(ms >> 16)
	u[4] = byte(ms >> 8)
	u[5] = byte(ms)

	u[6] = (u[6] & 0x0f) | 0x70 // version 7
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 9562 variant

	return u
}
