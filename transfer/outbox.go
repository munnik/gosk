// Package transfer delivers a vessel's mapped data to the central server
// and makes sure nothing is missing there, through an outbox.
//
// The vessel's mapped writer records, in the same transaction as every row
// it writes, which one-minute bucket of mapped_data the row falls in
// (database/outbox.go). The sender (`gosk transfer send`, send.go) reads
// pending buckets back from mapped_data and sends their rows to the far
// end; the receiver (`gosk transfer receive`, receive.go) stores them,
// waits for the commit, and acknowledges; the acknowledgement is the only
// thing that moves a bucket's acked_through forward. A bucket is pending
// while the far end has not acknowledged everything written into it, and
// a row that arrives late - older than something already sent from its
// bucket - resets the bucket so it is sent again from the start.
//
// Data written before a vessel was switched to this is handled by seeding
// (digest.go, seed.go, digest_respond.go): both ends summarise their
// history per bucket, and only the buckets that differ are put into the
// outbox for the sender to send.
//
// What this replaced: a protocol that asked the vessel how many rows it
// held per five-minute period, compared that with what had arrived, and
// re-requested the whole period when the numbers differed by more than a
// tolerated percentage. It needed a count per period on both ends forever,
// a continuous aggregate to produce them, a uuid per source message to
// narrow a difference down with, and a tolerance for permanent loss to
// stop itself spinning. The outbox needs the time column and nothing else,
// which is what let mapped_data.uuid be dropped.
package transfer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/munnik/gosk/message"
)

const (
	// dataTopic carries deliveries from a vessel to the far end, per
	// origin.
	dataTopic = "outbox/data/%s"
	// ackTopic carries the far end's confirmation that it has committed a
	// delivery, per origin.
	ackTopic = "outbox/ack/%s"
	// digestTopic carries a vessel's summary of part of its history while
	// seeding, and digestReplyTopic the far end's answer.
	digestTopic      = "outbox/digest/%s"
	digestReplyTopic = "outbox/digest-reply/%s"

	// QoS 1 throughout: the broker retransmits until the receiving end has
	// confirmed, and with a persistent session (mqtt.New) holds what was
	// published while the other end was away. A delivery that is lost
	// anyway is sent again after TransferConfig.RetryAfter.
	outboxQoS      = 1
	outboxRetained = false
)

// Delivery is one delivery: the rows of one bucket the far end does not
// have yet. Bucket, Generation and Through identify what the far end is
// acknowledging; Deltas carry the rows.
type Delivery struct {
	// Delivery identifies this attempt in the logs on both ends.
	Delivery   uuid.UUID        `json:"delivery"`
	Origin     string           `json:"origin"`
	Bucket     time.Time        `json:"bucket"`
	Generation int64            `json:"generation"`
	Through    time.Time        `json:"through"`
	Deltas     []message.Mapped `json:"deltas"`
	// Epoch is the run of the sender this comes from, reserved from the
	// far end when it started (see 20261020120000_outbox_epochs). The far
	// end orders a bucket's deliveries by epoch first and generation
	// second. A sender older than this field sends none, which reads as 0.
	Epoch int64 `json:"epoch,omitempty"`
}

// Ack is the far end saying it has committed a delivery. It echoes what
// the delivery named, and applies on the vessel only if the bucket is
// still on that generation.
type Ack struct {
	Delivery   uuid.UUID `json:"delivery"`
	Origin     string    `json:"origin"`
	Bucket     time.Time `json:"bucket"`
	Generation int64     `json:"generation"`
	Through    time.Time `json:"through"`
	// Stored is how many rows the far end wrote, for the logs. An
	// acknowledgement with Stored zero is still an acknowledgement: the
	// delivery had nothing left to send.
	Stored int `json:"stored"`
	// Epoch echoes the delivery's. A sender applies only acknowledgements
	// of its own run: one from an earlier run, delayed, can name the same
	// generation and a later through than anything this run has sent. Zero
	// from a far end older than epochs, which a sender takes as before.
	Epoch int64 `json:"epoch,omitempty"`
}

// groupRows folds rows read from mapped_data back into messages: rows
// that share a context, a timestamp and a source become one update with
// several values, which is the shape they were written in and a good deal
// smaller on the wire than one message per row.
func groupRows(rows []message.SingleValueMapped) []message.Mapped {
	type key struct {
		context string
		at      int64
		label   string
		type_   string
	}
	index := make(map[key]int, len(rows))
	result := make([]message.Mapped, 0, len(rows))
	for _, row := range rows {
		k := key{context: row.Context, at: row.Timestamp.UnixNano(), label: row.Source.Label, type_: row.Source.Type}
		m := row.ToMapped()
		if i, ok := index[k]; ok {
			// Appended, not AddValue'd: that replaces a value whose path
			// the update already has, and two rows here can share one.
			// A vessel's name and mmsi are stored under paths of their
			// own ("name", "mmsi", see mappedInsert) and read back under
			// the empty path they were written with, so a name-only and
			// an mmsi-only row from the same second and source would
			// collapse into one - and the one that was dropped would be
			// acknowledged as delivered all the same. Every row is a
			// distinct key in mapped_data, so there is nothing here to
			// merge.
			result[i].Updates[0].Values = append(result[i].Updates[0].Values, m.Updates[0].Values[0])
			continue
		}
		index[k] = len(result)
		result = append(result, *m)
	}
	return result
}

// codec compresses what goes onto the wire and works out for itself what
// is coming back off it.
//
// The existing writer and reader (writer/mqtt.go, reader/mqtt.go) each
// consult MQTTConfig.Compress and have to agree: set it on one side only
// and the other reads the payload as though it were JSON. That is a poor
// fit here, where one end is on a vessel and the other is in the cloud and
// they are upgraded weeks apart. What is read is therefore decided by the
// payload itself - a zstd frame announces itself - so the flag governs
// only what this process sends, and either end can be switched on its own.
type codec struct {
	encoder  *zstd.Encoder
	decoder  *zstd.Decoder
	compress bool
}

// zstdMagic is the frame header every zstd stream starts with, little
// endian 0xFD2FB528. JSON cannot begin with these bytes.
var zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}

// maxDecodedPayload bounds what one message may decode to. The far end
// decodes whatever any vessel publishes, the broker sets no size limit of
// its own, and a zstd frame of a few kilobytes can expand to gigabytes. A
// delivery of max_rows_per_delivery (20000 by default) rows is a few
// megabytes of JSON, so this leaves a wide margin.
const maxDecodedPayload = 64 << 20

func newCodec(compress bool) *codec {
	// The errors are for options that are invalid, which these are not.
	encoder, _ := zstd.NewWriter(nil)
	decoder, _ := zstd.NewReader(nil, zstd.WithDecoderMaxMemory(maxDecodedPayload))
	return &codec{encoder: encoder, decoder: decoder, compress: compress}
}

// encode marshals v, compressing it when this process is configured to.
// EncodeAll and DecodeAll are both safe to call from several goroutines at
// once, so one codec serves a whole process.
func (c *codec) encode(v any) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if !c.compress {
		return plain, nil
	}
	return c.encoder.EncodeAll(plain, make([]byte, 0, len(plain))), nil
}

// encodeSized is encode, and also returns how large the payload is once
// decoded: what the far end holds against maxDecodedPayload.
func (c *codec) encodeSized(v any) ([]byte, int, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, 0, err
	}
	if !c.compress {
		return plain, len(plain), nil
	}
	return c.encoder.EncodeAll(plain, make([]byte, 0, len(plain))), len(plain), nil
}

// decode unmarshals a payload, decompressing it first if it is compressed.
func (c *codec) decode(payload []byte, v any) error {
	if len(payload) > maxDecodedPayload {
		return fmt.Errorf("payload of %d bytes is larger than %d", len(payload), maxDecodedPayload)
	}
	if bytes.HasPrefix(payload, zstdMagic) {
		plain, err := c.decoder.DecodeAll(payload, nil)
		if err != nil {
			return fmt.Errorf("could not decompress the payload: %w", err)
		}
		payload = plain
	}
	return json.Unmarshal(payload, v)
}
