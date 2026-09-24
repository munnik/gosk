package transfer

import (
	"time"

	"github.com/munnik/gosk/message"
	"github.com/munnik/uuid/v5"
)

// The outbox scheme, section 4.2 of TRANSFER_REVIEW.md. It exists
// alongside the count-based request/respond protocol in this package
// rather than replacing it: a fleet cannot be cut over at once, and until
// every vessel ships this way the old scheme is what closes the gaps.
//
// What it replaces, once it does: the count protocol asks "how many rows
// do you have for this five minute period", compares that with what
// arrived, and re-requests the whole period when the numbers differ. It
// therefore needs a remote count for every period ever recorded, tolerates
// a fixed percentage of permanent loss to stop itself spinning, and can
// only ever answer in units of a period. The outbox instead names the
// source messages that have not been acknowledged - O(pending) state
// rather than O(origins x periods), exact rather than approximate, and
// nothing to tune.
//
// There is no watermark, and no sequence or timestamp column to hold one
// in. A message is pending if and only if it is in transfer_outbox. That
// is deliberately stronger than the "highest contiguous sequence" sketch
// in section 4.2: a watermark is only safe if the key it advances over is
// strictly increasing at the moment rows become visible, and rows here
// become visible when their transaction commits, which is not the order
// their uuids were generated in. A late commit under a watermark is a row
// silently skipped forever; a late commit under a pending set is just a
// row that gets sent slightly later.
//
// Ordering still matters, and comes from the uuids themselves: every one
// is a version 7 uuid, so oldest-first is uuid order.
const (
	// outboxDataTopic carries source messages from a vessel to the far
	// end. Per origin, as the count protocol's topics are.
	outboxDataTopic = "outbox/data/%s"
	// outboxAckTopic carries the far end's confirmation that it has
	// committed them.
	outboxAckTopic = "outbox/ack/%s"
)

// OutboxMessage is one shipment: the mapped data for a set of source
// messages, and the uuids identifying them.
//
// Uuids is not derivable from Deltas. A source message whose rows have
// since been dropped by a retention policy produces no deltas at all, and
// it still has to be acknowledged or it would sit in the outbox forever;
// listing the uuids separately is what lets the far end acknowledge
// something it received nothing for.
type OutboxMessage struct {
	// Shipment identifies this attempt, so a redelivery can be
	// recognised in the logs. It is not used for ordering.
	Shipment uuid.UUID        `json:"shipment"`
	Origin   string           `json:"origin"`
	Uuids    []uuid.UUID      `json:"uuids"`
	Deltas   []message.Mapped `json:"deltas"`
}

// OutboxAck is the far end saying it has committed a shipment. Only the
// uuids listed here are removed from the outbox, so a partial write
// acknowledges only what it actually stored.
type OutboxAck struct {
	Shipment uuid.UUID   `json:"shipment"`
	Origin   string      `json:"origin"`
	Uuids    []uuid.UUID `json:"uuids"`
	// Stored is how many rows the far end wrote, for the logs. An
	// acknowledgement with Stored zero is still an acknowledgement: the
	// source message had nothing left to send.
	Stored int `json:"stored"`
}

// outboxRetryAfter is how long a shipment may go unacknowledged before it
// is sent again. MQTT at QoS 1 redelivers the publish itself, so this
// covers what that cannot: the far end receiving a shipment and then
// failing to commit it.
const outboxRetryAfter = 5 * time.Minute
