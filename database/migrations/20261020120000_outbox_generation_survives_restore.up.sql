-- Generations that keep beating the far end's fence after this database is
-- restored.
--
-- The far end keeps, per (origin, bucket) and for good, the newest generation
-- it has stored (20261017120300_transfer_received), and refuses anything
-- older: that is what keeps a delayed delivery of an old generation from
-- overwriting a newer one. Generations used to come from
-- transfer_outbox_generation_seq alone. Restore this database from an earlier
-- snapshot and the sequence goes back with it, so every bucket it resets or
-- re-enqueues afterwards gets a generation the far end has already seen
-- exceeded - and the far end refuses those deliveries, without acknowledging
-- them, for as long as the sequence takes to catch up. Nothing on either end
-- would ever notice except a stale counter.
--
-- A generation is now at least the current time in microseconds, so one
-- given out after a restore is newer than any the previous incarnation gave
-- out before it, and anything that incarnation still has in flight stays
-- fenced. The sequence remains a floor and is moved up to every value given
-- out, so a clock that jumps backwards - a vessel booting without its time
-- yet - still cannot hand out a generation below the last one. Only both at
-- once, a restore and a clock behind the snapshot's, still could. Values
-- from before this migration are small sequence numbers, so the first
-- generations given out after it are all newer than everything stored, which
-- the far end takes as ordinary progress.
--
-- Two generations from this function can be equal across buckets; within one
-- bucket the callers take GREATEST of this and the bucket's previous
-- generation plus one, under the bucket's row lock, which is the property the
-- fence needs.
CREATE FUNCTION "transfer_outbox_next_generation"() RETURNS BIGINT
LANGUAGE plpgsql VOLATILE AS $$
DECLARE
    generation BIGINT;
BEGIN
    generation := GREATEST(
        nextval('transfer_outbox_generation_seq'),
        (extract(epoch FROM clock_timestamp()) * 1000000)::BIGINT
    );
    PERFORM setval('transfer_outbox_generation_seq', generation);
    RETURN generation;
END $$;

ALTER TABLE "transfer_outbox" ALTER COLUMN "generation" SET DEFAULT "transfer_outbox_next_generation"();
