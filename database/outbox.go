package database

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/munnik/gosk/message"
)

// The outbox, see 20261017120000_transfer_outbox and transfer/outbox.go.
//
// mapped_data is bucketed by time into one-minute buckets, and transfer_outbox
// holds one row per bucket that the far end has not acknowledged in full. The
// write path marks the bucket of every row it writes, in the same transaction
// as the row (see queuedQuery in postgresql.go); the sender reads the rows of a pending bucket
// back by time range and sends them; only an acknowledgement from the far end
// moves acked_through forward.
//
// OutboxBucket is fixed rather than configured because the sender, the
// seeder and the write path all have to agree on it, and they run as separate
// processes with separate configuration files: a mismatch would not fail, it
// would quietly over-send. One minute keeps a bucket at a few hundred to a
// few thousand rows on this fleet, which is one delivery, and bounds what a
// row arriving late costs - its whole bucket is sent again - to a minute of
// data.
const OutboxBucket = time.Minute

const (
	// outboxUpsertQuery marks a bucket as holding rows that are not yet
	// acknowledged. $3 is the earliest and $4 the latest row time written
	// into the bucket by one flush.
	//
	// The CASE is the late-row rule: a row no newer than something already
	// sent from this bucket cannot be sent as "what is newer than
	// acked_through", so the bucket starts over - a fresh generation, and
	// nothing sent or acknowledged. An acknowledgement in flight for the
	// old generation then applies to nothing. The comparison is against
	// sent_through rather than acked_through because the sender marks
	// what it has read before the far end has confirmed it, and a row that
	// slips in between the two would otherwise be covered by the
	// acknowledgement without ever having been sent.
	outboxUpsertQuery = `
		INSERT INTO "transfer_outbox" ("origin", "bucket", "latest")
		VALUES ($1, $2, $4)
		ON CONFLICT ("origin", "bucket") DO UPDATE SET
			"latest" = GREATEST("transfer_outbox"."latest", EXCLUDED."latest"),
			"generation" = CASE
				WHEN "transfer_outbox"."sent_through" IS NOT NULL AND $3 <= "transfer_outbox"."sent_through"
				THEN nextval('transfer_outbox_generation_seq')
				ELSE "transfer_outbox"."generation" END,
			"sent_through" = CASE
				WHEN "transfer_outbox"."sent_through" IS NOT NULL AND $3 <= "transfer_outbox"."sent_through"
				THEN NULL
				ELSE "transfer_outbox"."sent_through" END,
			"acked_through" = CASE
				WHEN "transfer_outbox"."sent_through" IS NOT NULL AND $3 <= "transfer_outbox"."sent_through"
				THEN NULL
				ELSE "transfer_outbox"."acked_through" END`

	outboxPendingWhere = `WHERE "acked_through" IS NULL OR "latest" > "acked_through"`

	outboxSelectOldestQuery = `SELECT "origin", "bucket", "generation", "latest", "acked_through" FROM "transfer_outbox" ` + outboxPendingWhere + ` ORDER BY "bucket" ASC LIMIT $1`
	outboxSelectNewestQuery = `SELECT "origin", "bucket", "generation", "latest", "acked_through" FROM "transfer_outbox" ` + outboxPendingWhere + ` ORDER BY "bucket" DESC LIMIT 1`
	outboxDepthQuery        = `SELECT "origin", COUNT(*) FROM "transfer_outbox" ` + outboxPendingWhere + ` GROUP BY 1`

	// outboxLockQuery takes the row lock the sender holds while it reads
	// a bucket's rows. A writer flushing rows into the same bucket blocks
	// on it until the sender has recorded what it read, so the writer's
	// late-row check (outboxUpsertQuery) runs against the sent_through
	// that covers this read, never against the one before it.
	outboxLockQuery = `SELECT "acked_through" FROM "transfer_outbox" WHERE "origin" = $1 AND "bucket" = $2 AND "generation" = $3 FOR UPDATE`
	//
	// Both watermarks only ever move forward within a generation (GREATEST
	// ignores a NULL): a generation spans several deliveries and their
	// retries, and MQTT at QoS 1 may hand the sender an old
	// acknowledgement again after a newer one. The ack query also matches
	// nothing when it would not advance, so its row count tells the sender
	// whether anything happened.
	outboxMarkSentQuery = `UPDATE "transfer_outbox" SET "sent_through" = GREATEST("sent_through", $4) WHERE "origin" = $1 AND "bucket" = $2 AND "generation" = $3`
	outboxAckQuery      = `UPDATE "transfer_outbox" SET "acked_through" = GREATEST("acked_through", $4) WHERE "origin" = $1 AND "bucket" = $2 AND "generation" = $3 AND ("acked_through" IS NULL OR "acked_through" < $4)`

	// outboxPruneQuery drops buckets that are acknowledged in full and
	// closed long enough ago that nothing is expected to land in them. A
	// late row into a pruned bucket recreates it from scratch, which costs
	// the whole bucket once - the same as a late row into a kept one.
	outboxPruneQuery = `DELETE FROM "transfer_outbox" WHERE "acked_through" IS NOT NULL AND "latest" <= "acked_through" AND "bucket" < NOW() - $1::interval`

	// outboxEnqueueRangeQuery puts every bucket in a time range into the
	// outbox as if nothing of it had been acknowledged. For seeding: the
	// far end has said it is missing some or all of the range.
	outboxEnqueueRangeQuery = `
		INSERT INTO "transfer_outbox" ("origin", "bucket", "latest")
		SELECT $1, public.time_bucket($4::interval, "time"), MAX("time")
		FROM "mapped_data"
		WHERE "origin" = $1 AND "time" >= $2 AND "time" < $3
		GROUP BY 2
		ORDER BY 2
		ON CONFLICT ("origin", "bucket") DO UPDATE SET
			"generation" = nextval('transfer_outbox_generation_seq'),
			"sent_through" = NULL,
			"acked_through" = NULL,
			"latest" = GREATEST("transfer_outbox"."latest", EXCLUDED."latest")`

	// mappedBetweenQuery reads what the sender sends: one bucket's rows
	// newer than what the far end has, oldest first. selectMappedQuery
	// has the column list.
	mappedBetweenQuery = ` WHERE "origin" = $1 AND "time" > $2 AND "time" < $3 ORDER BY "time" LIMIT $4`

	// digestQuery summarises a range of history per bucket so the two ends
	// can find where they differ without either sending the other its
	// rows. Per bucket: how many rows, and an exclusive or of a hash per
	// row, which describes the set without either side having to sort.
	//
	// The hash covers the columns that identify a row - the unique index
	// is (time, origin, context, path, connector), and origin is fixed by
	// the predicate - and not its value: a row both ends hold with
	// different values is not missing data, and comparing values would
	// make the comparison depend on how each end's jsonb happens to print.
	// The time goes in as integer microseconds since the epoch, which is
	// exact and the same on both ends whatever their session time zone.
	//
	// hashtextextended and bit_xor are core PostgreSQL, so both ends
	// compute the same number with nothing installed.
	digestQuery = `
		SELECT public.time_bucket($4::interval, "time") AS "bucket",
		       COUNT(*),
		       bit_xor(hashtextextended(concat_ws(E'\x1f', ((extract(epoch FROM "time") * 1000000)::bigint)::text, "context", "connector", "path"), 0))
		FROM "mapped_data"
		WHERE "origin" = $1 AND "time" >= $2 AND "time" < $3
		GROUP BY 1
		ORDER BY 1`

	earliestMappedQuery = `SELECT MIN("time") FROM "mapped_data" WHERE "origin" = $1`
)

// OutboxEntry is one pending bucket.
type OutboxEntry struct {
	Origin     string
	Bucket     time.Time
	Generation int64
	// Latest is the newest row time written into the bucket.
	Latest time.Time
	// AckedThrough is the newest row time the far end has confirmed, zero
	// when nothing of the bucket has been acknowledged.
	AckedThrough time.Time
}

// outboxMark is the bucket a queued mapped row has to be marked in: the
// row's origin and time (see queuedQuery).
type outboxMark struct {
	origin string
	time   time.Time
}

// outboxSpan is what one flush wrote into one bucket: a batch of ten
// thousand rows marks a handful of buckets rather than upserting ten
// thousand times.
type outboxSpan struct {
	earliest time.Time
	latest   time.Time
}

type outboxKey struct {
	origin string
	bucket time.Time
}

// outboxUpserts turns the marks of a flush's rows into one upsert per
// bucket. pgxBatch queues them behind the rows, in the same batch and so
// the same transaction.
//
// They are ordered by origin and bucket: a flush holds the locks of every
// bucket it marks until it commits, and the seeder's EnqueueOutboxRange
// locks every bucket of a range in one statement, in the same order, so the
// two cannot wait on each other crosswise.
func outboxUpserts(queries []queuedQuery) []queuedQuery {
	spans := map[outboxKey]outboxSpan{}
	order := []outboxKey{}
	for _, q := range queries {
		if q.mark == nil {
			continue
		}
		key := outboxKey{origin: q.mark.origin, bucket: q.mark.time.Truncate(OutboxBucket)}
		span, ok := spans[key]
		if !ok {
			spans[key] = outboxSpan{earliest: q.mark.time, latest: q.mark.time}
			order = append(order, key)
			continue
		}
		if q.mark.time.Before(span.earliest) {
			span.earliest = q.mark.time
		}
		if q.mark.time.After(span.latest) {
			span.latest = q.mark.time
		}
		spans[key] = span
	}
	slices.SortFunc(order, func(a, b outboxKey) int {
		if c := strings.Compare(a.origin, b.origin); c != 0 {
			return c
		}
		return a.bucket.Compare(b.bucket)
	})
	result := make([]queuedQuery, 0, len(order))
	for _, key := range order {
		span := spans[key]
		result = append(result, queuedQuery{sql: outboxUpsertQuery, args: []any{key.origin, key.bucket, span.earliest, span.latest}})
	}
	return result
}

// execWithMark writes one mapped row and the mark of its bucket in a
// transaction of its own, for retryIndividually. The mark comes after the
// row, as in the batch, and the two commit together: the late-row check in
// outboxUpsertQuery then runs against whatever the sender did in the
// meantime, and a row that commits into a bucket the sender has already
// read is sent again rather than settled unseen.
func (db *PostgresqlDatabase) execWithMark(ctx context.Context, q queuedQuery) error {
	tx, err := db.GetConnection().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
		return err
	}
	bucket := q.mark.time.Truncate(OutboxBucket)
	if _, err := tx.Exec(ctx, outboxUpsertQuery, q.mark.origin, bucket, q.mark.time, q.mark.time); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetOutbox turns the write path's outbox bookkeeping on or off, for a
// caller that builds its configuration in code rather than reading it - and
// for the tests, which cannot construct a second PostgresqlDatabase to get a
// differently configured one because the Prometheus collectors registered
// in NewPostgresqlDatabase panic on a duplicate name.
func (db *PostgresqlDatabase) SetOutbox(enabled bool) {
	db.batchMutex.Lock()
	defer db.batchMutex.Unlock()
	db.outboxEnabled = enabled
}

// SetBatchFlushLength changes how many queued queries trigger a flush, for
// the same reason as SetOutbox: a test that needs several writes in one
// batch cannot construct a database configured that way.
func (db *PostgresqlDatabase) SetBatchFlushLength(length int) {
	db.batchMutex.Lock()
	defer db.batchMutex.Unlock()
	db.batchSize = length
}

// SetTimeout changes how long a flush waits on the database, for a test
// that has one wait on a lock it holds itself.
func (db *PostgresqlDatabase) SetTimeout(timeout time.Duration) {
	db.databaseTimeout = timeout
}

// Flush writes everything queued now rather than at the next tick.
func (db *PostgresqlDatabase) Flush() {
	db.flushBatch()
	db.flushRawBatch()
}

// SelectPendingOutbox returns the newest pending bucket, if any, followed
// by up to limit of the oldest. Live data first: on a vessel that has just
// come back after a week, the current minute should not wait behind the
// backlog, and the backlog drains a few buckets per cycle behind it.
func (db *PostgresqlDatabase) SelectPendingOutbox(limit int) ([]OutboxEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()

	newest, err := db.selectOutbox(ctx, outboxSelectNewestQuery)
	if err != nil {
		return nil, err
	}
	oldest, err := db.selectOutbox(ctx, outboxSelectOldestQuery, limit)
	if err != nil {
		return nil, err
	}

	result := make([]OutboxEntry, 0, len(newest)+len(oldest))
	result = append(result, newest...)
	for _, e := range oldest {
		if len(newest) > 0 && e.Origin == newest[0].Origin && e.Bucket.Equal(newest[0].Bucket) {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

func (db *PostgresqlDatabase) selectOutbox(ctx context.Context, query string, arguments ...any) ([]OutboxEntry, error) {
	rows, err := db.GetConnection().Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []OutboxEntry{}
	for rows.Next() {
		var e OutboxEntry
		var acked *time.Time
		if err := rows.Scan(&e.Origin, &e.Bucket, &e.Generation, &e.Latest, &acked); err != nil {
			return nil, err
		}
		if acked != nil {
			e.AckedThrough = *acked
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// OutboxDepth is how many buckets are pending per origin.
func (db *PostgresqlDatabase) OutboxDepth() (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	rows, err := db.GetConnection().Query(ctx, outboxDepthQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var origin string
		var n int
		if err := rows.Scan(&origin, &n); err != nil {
			return nil, err
		}
		result[origin] = n
	}
	return result, rows.Err()
}

const (
	// receivedUpsertQuery records the generation of a bucket a delivery is
	// being stored from, unless a newer one was recorded already - in
	// which case it updates nothing, which is how StoreDelivery tells. The
	// conflicting row is locked either way.
	receivedUpsertQuery = `
		INSERT INTO "transfer_received" ("origin", "bucket", "generation", "received")
		VALUES ($1, $2, $3, now())
		ON CONFLICT ("origin", "bucket") DO UPDATE SET
			"generation" = EXCLUDED."generation",
			"received" = now()
		WHERE "transfer_received"."generation" <= EXCLUDED."generation"`
)

// ErrOutboxEntryGone is returned by ReadOutboxDelivery when the bucket is
// no longer pending under the generation the caller holds: a late row has
// reset it, or it was acknowledged and pruned. The caller simply picks it
// up again on its next pass, if it is still pending.
var ErrOutboxEntryGone = errors.New("the outbox entry is no longer pending under this generation")

// ReadOutboxDelivery reads what the far end does not have of one bucket -
// the rows newer than AckedThrough, at most limit of them - and records how
// far that read reached as sent_through, in one transaction.
//
// The returned through is the newest row time in the delivery, which is
// what the far end echoes in its acknowledgement. When the bucket holds
// more than limit rows the delivery stops at a whole timestamp: rows that
// share the last time read are left for the next delivery rather than
// split across two, because the next read starts strictly after through.
func (db *PostgresqlDatabase) ReadOutboxDelivery(entry OutboxEntry, limit int) (rows []message.SingleValueMapped, through time.Time, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()

	tx, err := db.GetConnection().Begin(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer tx.Rollback(ctx)

	var acked *time.Time
	if err := tx.QueryRow(ctx, outboxLockQuery, entry.Origin, entry.Bucket, entry.Generation).Scan(&acked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, ErrOutboxEntryGone
		}
		return nil, time.Time{}, err
	}

	// Strictly newer than what is acknowledged; for a bucket nothing has
	// been acknowledged of, everything from its start.
	after := entry.Bucket.Add(-time.Microsecond)
	if acked != nil {
		after = *acked
	}
	rows, err = db.readSingleValueMapped(ctx, tx, mappedBetweenQuery, entry.Origin, after, entry.Bucket.Add(OutboxBucket), limit+1)
	if err != nil {
		return nil, time.Time{}, err
	}
	if len(rows) == 0 {
		// Nothing newer than acked_through after all - latest ran ahead
		// of the rows, which a seeding run's upper bound can do. Record
		// latest as sent so the acknowledgement of an empty delivery
		// settles the bucket.
		through = entry.Latest
	} else if len(rows) > limit {
		cut := rows[limit].Timestamp
		kept := rows[:limit]
		for len(kept) > 0 && !kept[len(kept)-1].Timestamp.Before(cut) {
			kept = kept[:len(kept)-1]
		}
		if len(kept) == 0 {
			// Every one of the first limit rows shares the cut time. Send
			// all rows at that time together, however many there are.
			rows, err = db.readSingleValueMapped(ctx, tx, ` WHERE "origin" = $1 AND "time" = $2`, entry.Origin, cut)
			if err != nil {
				return nil, time.Time{}, err
			}
		} else {
			rows = kept
		}
		through = rows[len(rows)-1].Timestamp
	} else {
		through = rows[len(rows)-1].Timestamp
	}

	tag, err := tx.Exec(ctx, outboxMarkSentQuery, entry.Origin, entry.Bucket, entry.Generation, through)
	if err != nil {
		return nil, time.Time{}, err
	}
	if tag.RowsAffected() == 0 {
		return nil, time.Time{}, ErrOutboxEntryGone
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, time.Time{}, err
	}
	return rows, through, nil
}

// AckOutbox records that the far end has stored a bucket's rows up to and
// including through. It applies only to the generation the delivery was
// read under - a bucket reset by a late row in the meantime keeps its fresh
// generation and nothing acknowledged, and is sent again from the start -
// and only when it advances acked_through, so an old acknowledgement
// delivered again behind a newer one changes nothing. Returns whether it
// applied.
func (db *PostgresqlDatabase) AckOutbox(origin string, bucket time.Time, generation int64, through time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	tag, err := db.GetConnection().Exec(ctx, outboxAckQuery, origin, bucket, generation, through)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// PruneOutbox removes buckets acknowledged in full that closed more than
// olderThan ago.
func (db *PostgresqlDatabase) PruneOutbox(olderThan time.Duration) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()
	tag, err := db.GetConnection().Exec(ctx, outboxPruneQuery, olderThan)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// EnqueueOutboxRange marks every bucket of origin between from (inclusive)
// and to (exclusive) as entirely unacknowledged, so the sender sends all
// of it. For seeding; returns how many buckets hold rows in the range.
//
// The one statement locks every bucket of the range, in bucket order - the
// order a writer's flush takes its locks in too (outboxUpserts), so the two
// wait on each other in line rather than crosswise. Should they deadlock
// after all, Postgres aborts one after deadlock_timeout: the writer's side
// is then retried query by query (retryIndividually), this side is simply
// tried again.
func (db *PostgresqlDatabase) EnqueueOutboxRange(origin string, from, to time.Time) (int64, error) {
	const deadlockDetected = "40P01"
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout*10)
		var tag pgconn.CommandTag
		tag, err = db.GetConnection().Exec(ctx, outboxEnqueueRangeQuery, origin, from, to, OutboxBucket)
		cancel()
		if err == nil {
			return tag.RowsAffected(), nil
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != deadlockDetected {
			return 0, err
		}
	}
	return 0, err
}

// DigestBucket summarises one bucket of history: how many rows it holds
// and a hash of which rows they are. See digestQuery.
//
// JSON tags because this goes on the wire as it is: the whole point of a
// digest is that both ends compare the same numbers, so re-describing them
// in a second struct would only create somewhere for the two to drift.
type DigestBucket struct {
	Start time.Time `json:"start"`
	Rows  int64     `json:"rows"`
	Hash  int64     `json:"hash"`
}

// SelectDigest summarises everything this database holds for origin
// between from (inclusive) and to (exclusive), per bucket of width. Both
// ends run this same query over the same range at the same width, and
// compare what comes back. Buckets with no rows are absent.
func (db *PostgresqlDatabase) SelectDigest(origin string, from, to time.Time, width time.Duration) ([]DigestBucket, error) {
	// A seeding pass reads a lot more than a write does, and nobody is
	// waiting on it the way a writer's flush is waited on.
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout*10)
	defer cancel()

	rows, err := db.GetConnection().Query(ctx, digestQuery, origin, from, to, width)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []DigestBucket{}
	for rows.Next() {
		var b DigestBucket
		if err := rows.Scan(&b.Start, &b.Rows, &b.Hash); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// EarliestMapped is the time of the oldest mapped row this database holds
// for origin, and false when it holds none.
func (db *PostgresqlDatabase) EarliestMapped(origin string) (time.Time, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout*10)
	defer cancel()
	var earliest *time.Time
	if err := db.GetConnection().QueryRow(ctx, earliestMappedQuery, origin).Scan(&earliest); err != nil {
		return time.Time{}, false, err
	}
	if earliest == nil {
		return time.Time{}, false, nil
	}
	return *earliest, true, nil
}

// StoreDelivery writes the rows of a delivery of generation of the bucket
// in one transaction and returns only once they are committed, or with why
// they are not. It returns false, and stores nothing, when a newer
// generation of the bucket has been stored already: a late row on the
// vessel reset the bucket and the whole of it was sent and stored again
// while this delivery was still on its way, and storing it now would put
// its contents over the newer ones (see transfer_received's migration).
// The generation is recorded in the same transaction as the rows, and
// under the bucket's row lock, so what a restart or a second receiver on
// the same database sees is exactly what was stored. The same generation
// again is stored: that is a delivery sent again, or the next chunk of a
// bucket too large for one.
//
// Everything else here queues and returns, which is right for a writer
// whose job is to keep up with a sensor. It is wrong for anything that has
// to tell someone else the data is safe: the outbox receiver acknowledges a
// delivery to the vessel that sent it, and must not do so until the rows
// are committed. It also must not share the queued batch, whose periodic
// flush could pick these rows up, fail them quietly into the retry path,
// and leave nothing for a later Flush to report. The static_data updates
// that go with the rows (a vessel's name, say) are written in the same
// transaction for the same reason, and so that a delivery that is not
// stored leaves nothing behind - but one that postgres rejects is dropped
// on its own, as the vessel's writer dropped it (retryIndividually), and
// does not keep the rows it came with from being stored: the vessel has
// those rows, it has already given up on that update, and a delivery it
// could never get rid of would hold its bucket, and every row in it, back
// forever.
//
// No outbox bookkeeping, whatever the configuration: these rows arrived
// through the outbox, and marking them would send them back.
func (db *PostgresqlDatabase) StoreDelivery(origin string, bucket time.Time, generation int64, mapped []*message.Mapped) (bool, error) {
	batch := &pgx.Batch{}
	var statics []queuedQuery
	for _, m := range mapped {
		for _, svm := range m.ToSingleValueMapped() {
			query, arguments, static, ok := db.mappedInsert(svm)
			if !ok {
				continue
			}
			if static != nil {
				statics = append(statics, *static)
			}
			batch.Queue(query, arguments...)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), db.databaseTimeout)
	defer cancel()

	tx, err := db.GetConnection().Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	// Locks the bucket's row for the rest of the transaction, so that two
	// receivers storing deliveries of the same bucket into one database
	// take turns, and the one that goes second sees what the first
	// recorded.
	tag, err := tx.Exec(ctx, receivedUpsertQuery, origin, bucket, generation)
	if err != nil {
		return false, fmt.Errorf("could not record the delivery's generation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	for _, static := range statics {
		// A savepoint each: an error aborts the transaction otherwise,
		// and a value postgres will not take (a string where
		// static_data has a number) is to be dropped, not to fail the
		// delivery.
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return false, err
		}
		if _, err := savepoint.Exec(ctx, static.sql, static.args...); err != nil {
			if !db.dropIfUnwritable(static, err) {
				return false, fmt.Errorf("could not update static data: %w", err)
			}
			if err := savepoint.Rollback(ctx); err != nil {
				return false, fmt.Errorf("could not roll back a dropped static data update: %w", err)
			}
			continue
		}
		if err := savepoint.Commit(ctx); err != nil {
			return false, err
		}
	}

	if batch.Len() > 0 {
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return false, fmt.Errorf("could not write %d rows: %w", batch.Len(), err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
