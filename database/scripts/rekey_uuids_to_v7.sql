-- Rewrite every stored UUID as a version 7 UUID carrying the row's own
-- timestamp, deriving it from what the row already has.
--
-- This is a ONE-OFF, run by hand. It is deliberately not a migration:
-- migrations run automatically on every process start, and this rewrites
-- every row of the largest tables in the database.
--
--     psql "$GOSK_URL" -f rekey_uuids_to_v7.sql     -- installs, runs nothing
--     psql "$GOSK_URL" -c 'CALL public.rekey_uuids_to_v7()'
--
--
-- WHY
--
-- Rows written before gosk moved to version 7 UUIDs hold version 4 ones,
-- which carry no time at all; rows from four mappers held the nil UUID;
-- and rows written between the move to version 7 and the move to RFC 9562
-- method 3 carry a sub-millisecond fraction on a scale nothing reads
-- correctly. See sections 6 and 7 of TRANSFER_REVIEW.md.
--
--
-- THE CONVERSION IS A PURE FUNCTION
--
-- uuid_v7_at(time, uuid) depends on nothing but its two arguments: no
-- clock, no randomness, no sequence. Every vessel and the cloud therefore
-- derive the same new UUID for the same row without coordinating, which is
-- what makes it safe to run them independently.
--
-- It is also idempotent. The function keeps the low 60 bits of its input,
-- so applying it to its own output reproduces that output exactly:
--     uuid_v7_at(t, uuid_v7_at(t, u)) = uuid_v7_at(t, u)
-- That is what makes the script resumable and safe to re-run - a row
-- already converted is skipped by the `uuid <> uuid_v7_at(...)` predicate
-- rather than rewritten again.
--
-- Rows written by a current gosk are already fixed points, so a run after
-- the conversion finds nothing to do rather than churning fresh data. That
-- only holds because message.NewRaw truncates its clock reading to the
-- microsecond timestamptz stores: at nanosecond precision the UUID embeds
-- an instant the column cannot hold, and the recomputed UUID differs from
-- the stored one for most rows. Measured over 200 freshly written rows,
-- 200 were fixed points with the truncation and 53 without.
--
--
-- IMPACT, AND WHAT IT STILL COSTS
--
-- Work is done chunk by chunk and, within a chunk, a page range at a time,
-- committing after each batch. No long transaction, no table-wide lock held
-- for more than one batch, and a lock_timeout so a batch gives up rather
-- than queueing behind - or in front of - a live writer. Oldest chunks
-- first, so the cold data goes before the chunk currently being written to.
-- Progress is recorded per chunk, so it can be stopped with Ctrl-C and
-- resumed by calling it again.
--
-- It is still a rewrite: every converted row is a new row version, the
-- (origin, uuid, time) index is updated for each, and the WAL carries all
-- of it. Expect the table and its indexes to roughly double on disk until
-- autovacuum catches up. Run it when there is slack, and consider a manual
-- VACUUM afterwards.
--
--
-- BEFORE YOU RUN IT, ON A FLEET
--
-- The transfer protocol compares per-UUID counts between a vessel and the
-- cloud (SelectCountPerUuid). While one side is converted and the other is
-- not, the two see disjoint sets of UUIDs for the same period, and every
-- period reads as entirely missing - which would trigger a full
-- retransmission of everything. Either convert both ends close together
-- and accept a quiet period, or teach the comparison to apply
-- uuid_v7_at(time, uuid) on the unconverted side first. The purity of the
-- function is what makes that second option possible.
--
-- Note also that a mapped row and the raw row it came from share a UUID
-- today; after conversion they share one only where they also share a
-- timestamp. Rows whose time came from a payload timestampExpression, an
-- FFT window or an aggregate will no longer match their raw row. raw_data
-- is kept for seven days, so that link is short-lived either way.


-- ---------------------------------------------------------------------
-- The conversion
-- ---------------------------------------------------------------------

CREATE OR REPLACE FUNCTION public.uuid_v7_at(ts timestamptz, base uuid)
RETURNS uuid
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
RETURNS NULL ON NULL INPUT
AS $$
    SELECT (
        -- unix milliseconds, 48 bits. floor, not a bare cast: ::bigint
        -- rounds, and a timestamp in the last millisecond of a second
        -- would then get the next millisecond here while the fraction
        -- below still described the old one.
        lpad(to_hex(floor(extract(epoch FROM ts) * 1000)::bigint), 12, '0')
        -- version 7, then the sub-millisecond fraction in 4096ths of a
        -- millisecond - RFC 9562 method 3, the encoding PostgreSQL's own
        -- uuidv7() writes and TimescaleDB's uuid_timestamp_micros reads.
        || '7'
        || lpad(to_hex((((extract(microseconds FROM ts)::bigint % 1000) * 4096) / 1000)::int), 3, '0')
        -- the variant nibble, forced rather than inherited: taking it from
        -- base would carry the nil UUID's zero straight through and
        -- produce something that is not a valid UUID at all.
        || to_hex((('x' || substr(replace(base::text, '-', ''), 17, 1))::bit(4)::int & 3) | 8)
        -- and base's remaining 60 bits, untouched - so two rows that
        -- differ today still differ afterwards, and the row keeps whatever
        -- randomness it already had.
        || substr(replace(base::text, '-', ''), 18)
    )::uuid
$$;

COMMENT ON FUNCTION public.uuid_v7_at(timestamptz, uuid) IS
    'Pure, idempotent: a version 7 UUID embedding ts, keeping base''s random bits. Used by rekey_uuids_to_v7.';


-- ---------------------------------------------------------------------
-- Progress, so the run is resumable and reportable
-- ---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS public.uuid_rekey_progress (
    chunk          text PRIMARY KEY,
    parent_table   text NOT NULL,
    rows_rewritten bigint NOT NULL DEFAULT 0,
    skipped_reason text,
    started_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz
);

COMMENT ON TABLE public.uuid_rekey_progress IS
    'One row per chunk processed by rekey_uuids_to_v7. Drop it to force a full re-run; the conversion is idempotent so that is safe.';


-- ---------------------------------------------------------------------
-- The runner
-- ---------------------------------------------------------------------

CREATE OR REPLACE PROCEDURE public.rekey_uuids_to_v7(
    only_table          text     DEFAULT NULL,   -- NULL: all three
    pages_per_batch     int      DEFAULT 256,    -- 256 pages = 2MB at the default block size
    pause_per_batch     interval DEFAULT '0s',   -- breathing room for the live workload
    batch_lock_timeout  text     DEFAULT '5s'
)
LANGUAGE plpgsql
AS $$
DECLARE
    v_targets text[] := ARRAY['raw_data', 'mapped_data_matching_context', 'mapped_data_other_context'];
    v_parent  text;
    v_chunk   text;
    v_is_compressed bool;
    v_max_page bigint;
    v_start_page bigint;
    v_rewritten bigint;
    v_chunk_total bigint;
    v_grand_total bigint := 0;
BEGIN
    IF only_table IS NOT NULL THEN
        v_targets := ARRAY[only_table];
    END IF;

    FOREACH v_parent IN ARRAY v_targets LOOP
        IF to_regclass(v_parent) IS NULL THEN
            RAISE NOTICE 'skipping %, no such table here', v_parent;
            CONTINUE;
        END IF;

        -- Oldest chunks first: they are the cold ones. The newest chunk is
        -- where the live inserts are going, so it is left until last.
        FOR v_chunk, v_is_compressed IN
            SELECT format('%I.%I', c.chunk_schema, c.chunk_name), c.is_compressed
            FROM timescaledb_information.chunks c
            WHERE c.hypertable_name = v_parent
            ORDER BY c.range_start
        LOOP
            IF EXISTS (SELECT 1 FROM public.uuid_rekey_progress p
                       WHERE p.chunk = v_chunk AND p.finished_at IS NOT NULL) THEN
                CONTINUE;
            END IF;

            INSERT INTO public.uuid_rekey_progress (chunk, parent_table)
            VALUES (v_chunk, v_parent)
            ON CONFLICT (chunk) DO UPDATE SET started_at = now(), skipped_reason = NULL;
            COMMIT;

            IF v_is_compressed THEN
                -- A compressed chunk rejects UPDATE. Decompressing it here
                -- would be a far larger operation than this script is
                -- allowed to be, and would undo a compression policy's
                -- work behind its back.
                UPDATE public.uuid_rekey_progress
                SET skipped_reason = 'compressed', finished_at = now()
                WHERE chunk = v_chunk;
                COMMIT;
                RAISE WARNING 'skipped % - it is compressed, decompress it and re-run to include it', v_chunk;
                CONTINUE;
            END IF;

            EXECUTE format('SELECT pg_relation_size(%L) / current_setting(''block_size'')::bigint', v_chunk)
                INTO v_max_page;

            v_chunk_total := 0;
            v_start_page := 0;
            WHILE v_start_page <= v_max_page LOOP
                EXECUTE format('SET LOCAL lock_timeout = %L', batch_lock_timeout);

                -- By page range rather than LIMIT: each batch touches a
                -- bounded, distinct slice of the chunk, so the scan does
                -- not start over every time. Updated rows get new versions
                -- further along, and the predicate skips them when we
                -- reach them.
                EXECUTE format(
                    'UPDATE %s SET "uuid" = public.uuid_v7_at("time", "uuid")
                     WHERE ctid >= ''(%s,0)''::tid AND ctid < ''(%s,0)''::tid
                       AND "uuid" <> public.uuid_v7_at("time", "uuid")',
                    v_chunk, v_start_page, v_start_page + pages_per_batch
                );
                GET DIAGNOSTICS v_rewritten = ROW_COUNT;
                v_chunk_total := v_chunk_total + v_rewritten;

                COMMIT;

                IF pause_per_batch > interval '0' THEN
                    PERFORM pg_sleep(extract(epoch FROM pause_per_batch));
                END IF;

                v_start_page := v_start_page + pages_per_batch;
            END LOOP;

            UPDATE public.uuid_rekey_progress
            SET rows_rewritten = v_chunk_total, finished_at = now()
            WHERE chunk = v_chunk;
            COMMIT;

            v_grand_total := v_grand_total + v_chunk_total;
            RAISE NOTICE '% - % rows v_rewritten (% so far)', v_chunk, v_chunk_total, v_grand_total;
        END LOOP;
    END LOOP;

    RAISE NOTICE 'done, % rows v_rewritten in total', v_grand_total;
END;
$$;

COMMENT ON PROCEDURE public.rekey_uuids_to_v7(text, int, interval, text) IS
    'One-off: rewrites every uuid as a version 7 UUID embedding the row''s own time. Resumable, idempotent, v_chunk at a time.';
