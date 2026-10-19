-- The outbox: how a vessel knows what the central server has not stored yet.
--
-- One row per (origin, one-minute bucket of mapped_data.time), not per mapped
-- row and not per source message. The write path upserts the bucket of every
-- row it writes in the same transaction as the row itself, so a row can never
-- exist without its bucket being marked, and no bucket is marked for rows that
-- were not written. The sender (gosk transfer send) reads the rows of a
-- pending bucket from mapped_data by time range, sends them, and only the far
-- end's acknowledgement moves acked_through forward.
--
-- A bucket is pending while acked_through is NULL or latest is newer than
-- acked_through, and sending is incremental: rows newer than acked_through
-- are what gets sent, so the bucket that is still being written to does not
-- cost its whole contents on every delivery.
--
-- A row that arrives *late* - older than something already sent from its
-- bucket - cannot be expressed as "newer than acked_through", so it resets the
-- bucket: sent_through and acked_through go back to NULL and generation is
-- replaced. The far end's acknowledgement names the generation it is for, and
-- an acknowledgement for a generation that no longer exists does nothing, so a
-- late row can never be skipped by an acknowledgement that was in flight when
-- it arrived. generation comes from a sequence rather than a counter per row
-- for the same reason: a bucket that was deleted and then recreated by a late
-- row must not match an acknowledgement meant for its previous life.
--
-- The time column is partitioned on, so a bucket is a chunk-prunable range
-- on both ends, and the (time, origin, ...) unique index serves the read.
-- Nothing here needs mapped_data.uuid, which is what lets that column go.
CREATE SEQUENCE "transfer_outbox_generation_seq";

CREATE TABLE "transfer_outbox" (
    "origin"          TEXT NOT NULL,
    "bucket"          TIMESTAMP WITH TIME ZONE NOT NULL,
    "generation"      BIGINT NOT NULL DEFAULT nextval('transfer_outbox_generation_seq'),
    "latest"          TIMESTAMP WITH TIME ZONE NOT NULL,
    "sent_through" TIMESTAMP WITH TIME ZONE,
    "acked_through"   TIMESTAMP WITH TIME ZONE,
    PRIMARY KEY ("origin", "bucket")
);

COMMENT ON TABLE "transfer_outbox" IS
    'One-minute buckets of mapped_data the central server has not acknowledged in full. latest is the newest row time written into the bucket, acked_through the newest row time the far end confirmed; a bucket is pending while acked_through is NULL or older than latest.';

-- The sender wants the oldest pending bucket across origins and the newest
-- one (live data goes first), both of which the primary key's leading origin
-- does not serve.
CREATE INDEX "transfer_outbox_bucket_idx" ON "transfer_outbox" ("bucket");
