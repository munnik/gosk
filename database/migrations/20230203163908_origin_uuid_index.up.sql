CREATE INDEX IF NOT EXISTS "mapped_data_origin_uuid_time_idx" ON "mapped_data"("origin", "uuid", "time") WITH (timescaledb.transaction_per_chunk);
