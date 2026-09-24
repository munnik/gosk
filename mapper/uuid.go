package mapper

import (
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/uuid/v5"
	"go.uber.org/zap"
)

// uuidV7At returns a version 7 UUID whose embedded timestamp is t, keeping
// the random bits of base.
//
// It exists so that a row whose timestamp did not come from the raw
// message's arrival - see the timestampExpression handling in json.go -
// still carries a UUID that agrees with that timestamp. Without it, the
// UUID says when the bytes arrived and the row says when the measurement
// is for, and the two drift apart by however far the payload's clock is
// from ours.
//
// The timestamp bits, sub-millisecond fraction included, come from the
// uuid package's NewV7AtTimePrecise, so that encoding lives in one place
// rather than two. Its random bits are then replaced with base's, which
// does two things NewV7AtTimePrecise on its own would not:
//
//   - it keeps the result deterministic, so the same raw message with the
//     same payload timestamp always produces the same UUID, and
//     re-processing cannot silently change the identity of a row the
//     transfer protocol counts by UUID;
//   - it keeps the raw message's own randomness in the row, so two
//     messages reporting the same instant still differ.
//
// base is the raw message's UUID, itself a version 7 UUID, so those bits
// are already random.
func uuidV7At(t time.Time, base uuid.UUID) uuid.UUID {
	u, err := uuid.NewV7AtTimePrecise(t)
	if err != nil {
		// This only fails if the random source does, and the bits it drew
		// are the ones overwritten below in any case. The timestamp is
		// what was wanted, so fall back to the raw message's UUID rather
		// than losing the row over it.
		logger.GetLogger().Warn(
			"Could not derive a UUID for a payload timestamp, keeping the raw message's",
			zap.Time("Timestamp", t),
			zap.Error(err),
		)
		return base
	}

	copy(u[8:], base[8:])

	return u
}
