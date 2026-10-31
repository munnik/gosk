-- The uuid columns go.
--
-- raw_data.uuid and mapped_data.uuid were the link from a mapped row back to
-- the raw message it came from. Nothing ever read that link: no query joins
-- the two tables, raw_data expires after 7 days while mapped_data never does,
-- and row identity is the (time, origin, context, path, connector) unique
-- index, which does not include it. Its one real consumer was the count-based
-- transfer protocol, which grouped rows per uuid, and 20261031130000 dropped
-- that. transfer_uuid marked rows that arrived through the same protocol.
--
-- What it costs: measured on a compressed chunk, uuid was over half the
-- stored bytes of mapped_data - about N GB of the-production-server's N GB - and
-- 144 times what the time column costs, because a random 16 bytes does not
-- compress and an ordered timestamp almost disappears.
--
-- DROP COLUMN is a catalog change in PostgreSQL and in TimescaleDB's
-- columnstore alike: the column is marked dropped and skipped on read, and
-- no chunk, compressed or not, is rewritten. The space comes back as chunks
-- are recompressed or expire, not at once.
--
-- The mapped_data view is SELECT * over the two tables and so depends on
-- every column; it is dropped and recreated around the change. The index on
-- (origin, uuid, time) existed only for the transfer queries.
-- The view's grants are kept across that: people read gosk.mapped_data
-- directly (read access per person, granted by hand), and a recreated view
-- starts with none. They are saved before the drop and granted again after,
-- in this migration's transaction, so they cannot be lost half way. They
-- are carried in a transaction-local setting rather than a table: CREATE
-- TABLE AS on a database with the columnstore on crashes the server
-- (timescale/timescaledb#10655, see 20261031130300).
SELECT set_config('gosk.mapped_data_acl',
    COALESCE((SELECT "relacl"::text FROM "pg_catalog"."pg_class" WHERE "oid" = to_regclass('mapped_data')), ''),
    true);

DROP VIEW IF EXISTS "mapped_data";

DROP INDEX IF EXISTS "mapped_data_origin_uuid_time_idx";
DROP INDEX IF EXISTS "mapped_data_other_context_origin_uuid_time_idx";

ALTER TABLE "mapped_data_matching_context"
    DROP COLUMN IF EXISTS "uuid",
    DROP COLUMN IF EXISTS "transfer_uuid";

ALTER TABLE "mapped_data_other_context"
    DROP COLUMN IF EXISTS "uuid",
    DROP COLUMN IF EXISTS "transfer_uuid";

ALTER TABLE "raw_data"
    DROP COLUMN IF EXISTS "uuid";

CREATE VIEW "mapped_data" AS
    SELECT * FROM "mapped_data_matching_context"
    UNION ALL
    SELECT * FROM "mapped_data_other_context";

DO $$
DECLARE
    grant_ record;
BEGIN
    FOR grant_ IN
        SELECT acl.privilege_type, acl.grantee
        FROM aclexplode(NULLIF(current_setting('gosk.mapped_data_acl'), '')::aclitem[]) acl
        WHERE acl.grantee <> acl.grantor
    LOOP
        EXECUTE format('GRANT %s ON "mapped_data" TO %s', grant_.privilege_type,
            CASE WHEN grant_.grantee = 0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(grant_.grantee)) END);
    END LOOP;
END $$;
