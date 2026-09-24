-- 20230203163908_origin_uuid_index created "mapped_data_origin_uuid_time_idx"
-- on what was then the single "mapped_data" table.
-- 20231024113638_split_mapped_data renamed that table to
-- "mapped_data_matching_context" (which carried its indexes along) and
-- created "mapped_data_other_context" with
-- CREATE TABLE ... AS TABLE ... WITH NO DATA - which copies neither indexes
-- nor constraints. Only the unique index was then recreated explicitly, so
-- the index the transfer queries depend on has been missing from half the
-- mapped data ever since.
--
-- SelectCountPerUuid ("origin" = $1 AND "time" BETWEEN ...) and the
-- ReadMapped in the transfer responder both go through the "mapped_data"
-- view, which is a UNION ALL over both tables - so every one of those
-- queries has been paying for a scan of the other_context half.
CREATE INDEX IF NOT EXISTS "mapped_data_other_context_origin_uuid_time_idx"
ON "mapped_data_other_context"("origin", "uuid", "time")
WITH (timescaledb.transaction_per_chunk);
