// Package uuidv7 is the single place gosk generates UUIDs, so that they
// are all version 7.
//
// Version 4 is 122 bits of randomness, which makes consecutive values land
// in unrelated places. Version 7 puts a 48 bit millisecond timestamp in
// the high bits, so values generated in sequence sort in the order they
// were created. Everything below follows from that ordering:
//
//   - Index locality. Every raw message carries a UUID (message.NewRaw)
//     that is written to raw_data and follows the mapping into
//     mapped_data, where "mapped_data_origin_uuid_time_idx" indexes it.
//     Random UUIDs scatter each insert to a different part of that index,
//     so the pages being written are spread across the whole of it - more
//     page splits, and a write working set far larger than the data
//     actually being inserted. Ordered UUIDs append near one edge.
//   - Compression. A batch of ordered UUIDs shares its leading bytes;
//     random ones share nothing. This only pays off on chunks that are
//     actually compressed, which no migration currently sets up.
//
// Neither is retroactive: rows already written keep their version 4 UUIDs,
// so an index stays as fragmented as it is until those chunks age out.
// raw_data turns over within its 7 day retention; mapped_data has no
// retention policy and will be mixed for a long time.
//
// The trade is that a version 7 UUID is no longer opaque - it discloses
// when it was created, to the millisecond. Everywhere gosk stores one it
// sits next to a "time" column holding that same information, so nothing
// is disclosed that was not already there.
package uuidv7

import "github.com/google/uuid"

// New returns a version 7 UUID, panicking if the system's source of
// randomness fails - the same behaviour, from the same cause, as
// uuid.New's for version 4.
//
// Values are strictly increasing, including within a millisecond: the
// implementation keeps a sequence counter in the 12 bits below the
// timestamp. That ordering is per process, so two processes writing to one
// table interleave at millisecond granularity, which is as much as the
// index locality above needs.
func New() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}
