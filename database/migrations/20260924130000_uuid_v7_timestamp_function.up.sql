-- public.uuid_v7_timestamp(uuid) returns the creation time embedded in a
-- version 7 UUID, and NULL for any other version.
--
-- This is a debugging aid, not something gosk queries: mapped_data.time
-- and mapped_data.uuid answer different questions (see section 7 of
-- TRANSFER_REVIEW.md), and comparing "when did this raw message arrive"
-- against "when is this measurement for" is exactly the discrepancy the
-- count-based transfer reconciliation cannot see.
--
-- PostgreSQL grew uuid_extract_timestamp in 17, but that version handles
-- version 1 only and returns NULL for version 7 - silently, while
-- uuid_extract_version on the same value correctly answers 7. Version 7
-- support arrived in 18. So the body is chosen here, once, by asking the
-- server what it can actually do rather than by checking a version
-- number, and the name stays the same either way so queries and
-- dashboards do not have to care.
DO $migration$
DECLARE
    builtin_handles_v7 boolean := false;
BEGIN
    IF to_regprocedure('pg_catalog.uuid_extract_timestamp(uuid)') IS NOT NULL THEN
        EXECUTE $probe$
            SELECT pg_catalog.uuid_extract_timestamp(
                '01a0d440-ef27-71e9-a3ef-512d5239ffb2'::uuid
            ) IS NOT NULL
        $probe$ INTO builtin_handles_v7;
    END IF;

    IF builtin_handles_v7 THEN
        EXECUTE $create$
            CREATE OR REPLACE FUNCTION public.uuid_v7_timestamp(u uuid)
            RETURNS timestamptz
            LANGUAGE sql
            IMMUTABLE
            PARALLEL SAFE
            RETURNS NULL ON NULL INPUT
            AS $body$
                -- Guarded on the version so this matches the fallback
                -- below, which cannot tell a version 1 timestamp from a
                -- version 7 one and so reports neither.
                SELECT CASE
                    WHEN pg_catalog.uuid_extract_version(u) = 7
                    THEN pg_catalog.uuid_extract_timestamp(u)
                END
            $body$
        $create$;
        RAISE NOTICE
            'uuid_v7_timestamp delegates to the built-in uuid_extract_timestamp';
    ELSE
        EXECUTE $create$
            CREATE OR REPLACE FUNCTION public.uuid_v7_timestamp(u uuid)
            RETURNS timestamptz
            LANGUAGE sql
            IMMUTABLE
            PARALLEL SAFE
            RETURNS NULL ON NULL INPUT
            AS $body$
                -- The first 48 bits are the unix timestamp in
                -- milliseconds, big endian; the 13th hex digit is the
                -- version. Left-pad to 16 hex digits because the
                -- bit(64)->bigint cast is the reliable width.
                SELECT CASE
                    WHEN substr(u::text, 15, 1) <> '7' THEN NULL
                    ELSE to_timestamp(
                        (
                            'x' || lpad(
                                substr(u::text, 1, 8) || substr(u::text, 10, 4),
                                16,
                                '0'
                            )
                        )::bit(64)::bigint / 1000.0
                    )
                END
            $body$
        $create$;
        RAISE NOTICE
            'uuid_v7_timestamp extracts the timestamp itself, this server''s uuid_extract_timestamp does not handle version 7';
    END IF;
END
$migration$;

COMMENT ON FUNCTION public.uuid_v7_timestamp(uuid) IS
    'Creation time embedded in a version 7 UUID, NULL for other versions. Millisecond resolution - two UUIDs generated within one millisecond report the same time.';
