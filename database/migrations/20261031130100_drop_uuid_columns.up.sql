-- The uuid columns go.
--
-- raw_data.uuid and mapped_data.uuid were the link from a mapped row back to
-- the raw message it came from. Nothing ever read that link: no query joins
-- the two tables, raw_data expires after 7 days while mapped_data never does,
-- and row identity is the (time, origin, context, path, connector) unique
-- index, which does not include it. Its one real consumer was the count-based
-- transfer protocol, which grouped rows per uuid, and 20261017120100 dropped
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
